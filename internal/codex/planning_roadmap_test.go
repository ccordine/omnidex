package codex

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// Qualify semantic extraction through the real parser, without asking it to
// design the plan or giving it the expected task count. The normal CLI pilot
// separately exercises the planner that produces this kind of roadmap.
func TestInstalledPlanningRoadmapBoundaries(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	run := func(call codingagent.Assignment) json.RawMessage {
		t.Helper()
		call.JobID, call.Generation, call.Root, call.Route = 17, 1, t.TempDir(), route
		result, err := provider.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		return result.Content
	}
	var parsed struct {
		Ledger codingagent.EngineeringLedger `json:"ledger"`
	}
	raw := run(codingagent.Assignment{Role: codingagent.PlanningInitial, PlanningResponse: `Roadmap: local invoice management.

Milestone: invoice workflows (extend the existing cli.go and store.go).
- Deliverable: invoice creation. Persist an invoice with customer, amount and due date. Done when the real CLI can create and reload it after restart. Include validation tests, a restart check and a README usage example in this task.
- Deliverable: overdue filtering. List unpaid invoices whose due date has passed, preserving stored invoices. Done when the real CLI shows an overdue fixture and excludes paid and future fixtures. Include those tests and its README example in this task.

Both deliverables use the same database. Preserve existing records. No external services, deployment or other features are proposed.`})
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Ledger.Validate(); err != nil {
		t.Fatal(err)
	}
	assertRoadmapTasks(t, parsed.Ledger.Tasks, []string{"invoice creation", "overdue filtering"})

	var change struct {
		Operations []model.LedgerMutationOperation `json:"operations"`
	}
	raw = run(codingagent.Assignment{Role: codingagent.PlanningMutation, PlanningLedger: &parsed.Ledger,
		PlanningResponse: `Keep both existing tasks and all their details unchanged. Extend the same invoice roadmap with these deliverables, using the same cli.go and store.go:

Reporting milestone:
- Deliverable: CSV export. Export stored invoices with a header and correctly escaped fields. Done when the real command exports a comma-containing fixture and a CSV reader recovers the original values. Include regression tests and usage documentation here.
- Deliverable: reminder preview. Print a local reminder for a selected unpaid invoice without sending it. Done when the real command displays the selected invoice's customer, amount and due date, with no network access or record mutation. Include regression tests and usage documentation here.

These are additions, not subtasks of invoice creation. Preserve existing records. No delivery provider or deployment is proposed.`})
	if err := json.Unmarshal(raw, &change); err != nil {
		t.Fatal(err)
	}
	var added []model.LedgerItemID
	for _, operation := range change.Operations {
		if operation.Kind != model.LedgerMutationAdd || operation.Item == nil || operation.Item.Task == nil {
			t.Fatalf("expected only new deliverables, got %+v", operation)
		}
		added = append(added, operation.Item.Task.ID)
	}
	next, err := codingagent.ApplyPlanningMutation(parsed.Ledger, codingagent.LedgerMutation{
		BaseRevision: parsed.Ledger.Revision, Operations: change.Operations,
		OwnerTurnID: "roadmap-amendment", OperationID: "roadmap-amendment",
		OwnerTurnDisposition: model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{{
			Kind: model.OwnerTurnRepresented, ItemIDs: added,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRoadmapTasks(t, next.Tasks, []string{"invoice creation", "overdue filtering", "csv export", "reminder preview"})
	for _, original := range parsed.Ledger.Tasks {
		var retained *codingagent.LedgerTask
		for i := range next.Tasks {
			if next.Tasks[i].ID == original.ID {
				retained = &next.Tasks[i]
			}
		}
		if retained == nil || !reflect.DeepEqual(original, *retained) {
			t.Fatalf("amendment changed an existing task: %s", original.ID)
		}
	}
}

func assertRoadmapTasks(t *testing.T, tasks []codingagent.LedgerTask, deliverables []string) {
	t.Helper()
	if len(tasks) != len(deliverables) {
		t.Fatalf("roadmap has %d tasks, want %d: %+v", len(tasks), len(deliverables), tasks)
	}
	for _, name := range deliverables {
		matches := 0
		for _, task := range tasks {
			if strings.Contains(strings.ToLower(task.Goal+" "+task.Objective), name) {
				matches++
				if len(task.Acceptance) == 0 {
					t.Fatalf("deliverable %q lost its completion criteria", name)
				}
			}
		}
		if matches != 1 {
			t.Fatalf("deliverable %q appears in %d tasks, want its own task: %+v", name, matches, tasks)
		}
	}
}
