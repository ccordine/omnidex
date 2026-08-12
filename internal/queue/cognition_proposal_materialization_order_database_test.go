package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

func TestPostgresCognitionProposalMaterializationPreservesEveryOrderedProposal(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "063")); err != nil {
		t.Fatal(err)
	}
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	decision := cognitionProposalMaterializationDecision(fixture)
	decision.Proposals = []cognition.LedgerProposal{
		{Kind: cognition.ProposalObservation, Content: "A public mechanism was observed.", EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence}},
		{Kind: cognition.ProposalHypothesis, Content: "The public mechanism may remain available.", EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence}},
		{Kind: cognition.ProposalQuestion, Content: "Does the public mechanism remain available?"},
	}
	bound := buildCognitionDecisionStep(t, fixture, decision)
	receipt, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), `
		SELECT proposal_index,proposal_kind,source_kind,output_ledger_version
		FROM cognition_proposal_materializations WHERE episode_id=$1 ORDER BY proposal_index
	`, fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	wantKinds := []string{"observation", "hypothesis", "question"}
	wantSources := []cognitionstate.SourceKind{
		cognitionstate.SourceModelObservation,
		cognitionstate.SourceModelHypothesis,
		cognitionstate.SourceModelQuestion,
	}
	count := 0
	for rows.Next() {
		var index int
		var kind, source string
		var version uint64
		if err := rows.Scan(&index, &kind, &source, &version); err != nil {
			t.Fatal(err)
		}
		if index != count || kind != wantKinds[count] || source != string(wantSources[count]) ||
			version+uint64(len(wantKinds)-count-1) != receipt.LedgerVersion {
			t.Fatalf("ordered proposal %d = %d/%q/%q/v%d receipt=v%d", count, index, kind, source, version, receipt.LedgerVersion)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(wantKinds) {
		t.Fatalf("proposal materialization count = %d want %d", count, len(wantKinds))
	}
}
