package queue

import (
	"strconv"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type cognitionProposalMaterializationSetFixture struct {
	Members   []CognitionProposalMaterializationTraceMember
	Authority CognitionProposalMaterializationReconciliationAuthority
	Snapshot  cognition.RuntimeSnapshot
	Command   cognitionruntime.ReconciliationCommand
	Receipt   cognitionruntime.ReconciliationReceipt
}

func TestPostgresCognitionProposalMaterializationPortableSetRequiresEveryProposal(t *testing.T) {
	fixture := newCognitionProposalMaterializationSetFixture(t)
	if err := VerifyCognitionProposalMaterializationTraceSet(
		fixture.Members, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt,
	); err != nil {
		t.Fatalf("verify exact proposal materialization set: %v", err)
	}
	if err := VerifyCognitionProposalMaterializationTraceSet(
		fixture.Members[:1], fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt,
	); err == nil {
		t.Fatal("portable set verifier accepted an omitted proposal materialization")
	}
}

func newCognitionProposalMaterializationSetFixture(
	t *testing.T,
) cognitionProposalMaterializationSetFixture {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "063")); err != nil {
		t.Fatal(err)
	}
	database := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), database.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	decision := cognitionProposalMaterializationDecision(database)
	decision.Proposals = append(decision.Proposals, cognition.LedgerProposal{
		Kind: cognition.ProposalQuestion, Content: "Which exact mechanism remains unresolved?",
	})
	bound := buildCognitionDecisionStep(t, database, decision)
	receipt, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(t.Context(), `
		SELECT payload_json,payload_json_sha256
		FROM cognition_proposal_materializations
		WHERE reconciliation_id=$1 ORDER BY proposal_index
	`, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	members := make([]CognitionProposalMaterializationTraceMember, 0, len(decision.Proposals))
	for rows.Next() {
		var raw []byte
		var payloadSHA string
		if err := rows.Scan(&raw, &payloadSHA); err != nil {
			t.Fatal(err)
		}
		value, err := DecodeCognitionProposalMaterialization(raw, payloadSHA)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, CognitionProposalMaterializationTraceMember{
			Value: value,
			Authority: CognitionProposalMaterializationTraceAuthority{
				ReconciliationID: value.ReconciliationID, PolicyCallID: value.PolicyCallID,
				CallOrdinal: value.CallOrdinal, Phase: CognitionProposalMaterializationTracePhase,
				Sequence: int64(value.ProposalIndex), ID: value.ID, SHA256: payloadSHA,
			},
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(members) != len(decision.Proposals) {
		t.Fatalf("materialization members=%d want=%d", len(members), len(decision.Proposals))
	}
	return cognitionProposalMaterializationSetFixture{
		Members: members,
		Authority: CognitionProposalMaterializationReconciliationAuthority{
			ReconciliationID: receipt.ID, PolicyCallID: members[0].Value.PolicyCallID,
			CallOrdinal: members[0].Value.CallOrdinal,
		},
		Snapshot: bound.Prepared.Prepared.Snapshot, Command: bound.Command, Receipt: receipt,
	}
}

func cognitionProposalRevisionCommand(
	t *testing.T, fixture cognitionProposalMaterializationSetFixture,
) (cognitionruntime.ReconciliationCommand, cognitionruntime.ReconciliationReceipt) {
	t.Helper()
	command := fixture.Command.Clone()
	command.Decision.Proposals = []cognition.LedgerProposal{{
		Kind: cognition.ProposalRevision,
		Revision: &cognition.BeliefRevisionProposal{
			TargetRef: cognition.EpistemicRef{
				URI:     fixture.Members[0].Value.EntryURI,
				Version: strconv.FormatUint(fixture.Members[0].Value.OutputLedgerVersion, 10),
				SHA256:  cognitionTestDigest("f"),
			},
			EvidenceRefs: []cognition.EvidenceRef{fixture.Command.Decision.EvidenceRefs[0]},
		},
	}}
	receipt, err := cognitionruntime.NewReconciliationReceipt(
		command, fixture.Receipt.LedgerVersion, fixture.Receipt.WorkingSetVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	return command, receipt
}
