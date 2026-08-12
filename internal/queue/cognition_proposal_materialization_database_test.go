package queue

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionProposalMaterializationIsExactAndReplayable(t *testing.T) {
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
	bound := buildCognitionDecisionStep(t, fixture, decision)
	receipt, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil || replayed != receipt {
		t.Fatalf("proposal materialization replay=%#v want=%#v error=%v", replayed, receipt, err)
	}
	var (
		id, selfSHA, payloadSHA, proposalKind, sourceKind  string
		preLedgerSHA, outputStatus                         string
		callOrdinal, preLedgerVersion, outputLedgerVersion int64
		proposalIndex                                      int
		raw                                                []byte
	)
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT materialization_id,materialization_sha256,payload_json_sha256,payload_json,
		       call_ordinal,proposal_index,proposal_kind,source_kind,
		       pre_proposal_ledger_version,pre_proposal_ledger_sha256,
		       output_ledger_version,output_ledger_status
		FROM cognition_proposal_materializations WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(
		&id, &selfSHA, &payloadSHA, &raw, &callOrdinal, &proposalIndex,
		&proposalKind, &sourceKind, &preLedgerVersion, &preLedgerSHA,
		&outputLedgerVersion, &outputStatus,
	); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Schema                      string                          `json:"schema"`
		ID                          string                          `json:"id"`
		SHA256                      string                          `json:"sha256"`
		EpisodeID                   cognition.EpisodeID             `json:"episode_id"`
		ReconciliationID            string                          `json:"reconciliation_id"`
		PolicyCallID                string                          `json:"policy_call_id"`
		CallOrdinal                 uint64                          `json:"call_ordinal"`
		SnapshotSHA256              string                          `json:"snapshot_sha256"`
		DecisionSHA256              string                          `json:"decision_sha256"`
		ProposalIndex               int                             `json:"proposal_index"`
		Proposal                    cognition.LedgerProposal        `json:"proposal"`
		SourceKind                  cognitionstate.SourceKind       `json:"source_kind"`
		PreProposalLedgerVersion    uint64                          `json:"pre_proposal_ledger_version"`
		PreProposalLedgerSHA256     string                          `json:"pre_proposal_ledger_sha256"`
		PreProposalLedgerJSONSHA256 string                          `json:"pre_proposal_ledger_json_sha256"`
		PreProposalLedger           taskstate.MaterializedState     `json:"pre_proposal_ledger"`
		ReplayDescriptor            cognitionstate.ReplayDescriptor `json:"replay_descriptor"`
		Command                     taskstate.AddEntryCommand       `json:"command"`
		EntryURI                    string                          `json:"entry_uri"`
		OutputLedgerVersion         uint64                          `json:"output_ledger_version"`
		OutputLedgerStatus          taskstate.LedgerStatus          `json:"output_ledger_status"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	canonical, err := exactjson.Canonical(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, canonical) || cognitionPayloadSHA(raw) != payloadSHA {
		t.Fatal("proposal materialization payload is not exact canonical trace authority")
	}
	if payload.ID != id || payload.SHA256 != selfSHA ||
		!strings.HasPrefix(id, "cognition_proposal_materialization_") ||
		payload.EpisodeID != fixture.EpisodeID || payload.CallOrdinal != uint64(callOrdinal) ||
		payload.ProposalIndex != proposalIndex || proposalIndex != 0 ||
		payload.Proposal.Kind != cognition.ProposalHypothesis || proposalKind != string(payload.Proposal.Kind) ||
		payload.SourceKind != cognitionstate.SourceModelHypothesis || sourceKind != string(payload.SourceKind) ||
		payload.PreProposalLedgerVersion != uint64(preLedgerVersion) || payload.PreProposalLedgerSHA256 != preLedgerSHA ||
		payload.ReplayDescriptor.LedgerSHA256 != preLedgerSHA ||
		payload.ReplayDescriptor.ExpectedVersion != uint64(preLedgerVersion) ||
		payload.OutputLedgerVersion != uint64(outputLedgerVersion) ||
		payload.OutputLedgerVersion != payload.Command.ExpectedVersion+1 ||
		payload.OutputLedgerStatus != taskstate.LedgerActive || outputStatus != string(taskstate.LedgerActive) ||
		payload.EntryURI != "task:ledger/"+string(payload.ReplayDescriptor.LedgerID)+"/entry/"+string(payload.Command.ID) {
		t.Fatalf("proposal materialization=%#v columns=%q/%q/%d/%d", payload, proposalKind, sourceKind, preLedgerVersion, outputLedgerVersion)
	}
	if err := payload.ReplayDescriptor.Validate(payload.Command); err != nil {
		t.Fatalf("proposal materialization replay descriptor: %v", err)
	}
}
