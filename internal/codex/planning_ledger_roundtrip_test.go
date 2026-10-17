package codex

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

type roundTripEvidence struct{ entries []codingagent.Entry }

func (e *roundTripEvidence) Append(_ context.Context, entry codingagent.Entry) (int64, error) {
	entry.ID = int64(len(e.entries) + 1)
	e.entries = append(e.entries, entry)
	return entry.ID, nil
}
func (e *roundTripEvidence) Read(context.Context) ([]codingagent.Entry, error) {
	return append([]codingagent.Entry(nil), e.entries...), nil
}
func (e *roundTripEvidence) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}

type roundTripEnvironment struct{}

func (roundTripEnvironment) Attest(context.Context) error { return nil }

func TestInstalledPlanningLedgerRoundTrip(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	evidence := &roundTripEvidence{}
	controller := &codingagent.Controller{
		JobID: 17, Generation: 1, Root: t.TempDir(),
		Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
		Providers:     map[string]codingagent.Provider{"codex": provider}, Evidence: evidence, Environment: roundTripEnvironment{},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()
	turn := codingagent.PlanningTurn{
		OperationID: "roundtrip-initial", OwnerTurnID: "roundtrip-owner-initial",
		Text: "Plan a small local Python standard-library command-line expense tracker with persistence, validation, automated tests, and a README. Do not modify Omnidex.",
	}
	proposal, err := controller.ProposePlanningLedger(ctx, turn)
	if err != nil {
		t.Fatal(err)
	}
	if err := proposal.Ledger.Validate(); err != nil {
		t.Fatalf("initial parsed ledger is invalid: %v", err)
	}
	if len(proposal.Ledger.Tasks) == 0 {
		t.Fatal("initial round trip produced no tasks")
	}
	mutationTurn := codingagent.PlanningTurn{
		OperationID: "roundtrip-mutation", OwnerTurnID: "roundtrip-owner-mutation",
		Text: "Add a README usage example and preserve every existing task exactly.", ObservedRevision: proposal.Ledger.Revision,
	}
	mutation, err := controller.PlanPlanningMutation(ctx, proposal.Ledger, mutationTurn)
	if err != nil {
		t.Fatal(err)
	}
	next, err := codingagent.ApplyPlanningMutation(proposal.Ledger, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Validate(); err != nil {
		t.Fatalf("mutated parsed ledger is invalid: %v", err)
	}
	if len(next.Tasks) < len(proposal.Ledger.Tasks) {
		t.Fatalf("mutation discarded tasks: before=%d after=%d", len(proposal.Ledger.Tasks), len(next.Tasks))
	}
	for _, task := range next.Tasks {
		if task.ID == "" || task.Goal == "" {
			t.Fatalf("round-trip task lost code-owned identity or semantic goal: %+v", task)
		}
	}
	if encoded, err := json.Marshal(next); err != nil || len(encoded) == 0 {
		t.Fatalf("round-trip ledger was not serializable: %v", err)
	}
}
