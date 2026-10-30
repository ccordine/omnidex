package codex

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestInstalledPlanningStatusDoesNotReopenCompletedWork(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	ledger := codingagent.EngineeringLedger{ID: "ledger", Revision: 4, Tasks: []codingagent.LedgerTask{
		{LedgerItem: model.LedgerItem{ID: "invoice-import", Rank: 1, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed},
			Goal: "Import invoices", Objective: "Import valid CSV invoices without changing existing records."},
		{LedgerItem: model.LedgerItem{ID: "invoice-search", Rank: 2, Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewAgreed},
			Goal: "Search invoices", Objective: "Filter invoices by customer name."},
	}}
	for _, test := range []struct {
		name, response string
		wantChanges    int
	}{
		{"status-only", "Retain invoice-import as completed. The accepted fixture imported three valid invoices, rejected invalid rows and preserved previous records. Independent review reports no findings. All identities, instructions and ordering remain unchanged. This turn reviews existing evidence only.", 0},
		{"status-with-real-amendment", "Retain invoice-import as completed, with its existing contract and passing review unchanged. Change only invoice-search: its objective is to filter invoices by customer name and due date. All other instructions and task details remain unchanged.", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := codingagent.Assignment{Role: codingagent.PlanningMutation, JobID: 17, Generation: 1,
				Root: t.TempDir(), Route: route, PlanningLedger: &ledger,
				CompletedTaskIDs: []string{"invoice-import"}, PlanningResponse: test.response}
			result, err := provider.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			var parsed struct {
				Operations []model.LedgerMutationOperation `json:"operations"`
			}
			if err := json.Unmarshal(result.Content, &parsed); err != nil {
				t.Fatal(err)
			}
			if len(parsed.Operations) != test.wantChanges {
				t.Fatalf("status report became work or a real change was lost: %s", result.Content)
			}
			for _, operation := range parsed.Operations {
				if operation.Kind != model.LedgerMutationAmend || operation.TargetID != "invoice-search" || operation.Item == nil || operation.Item.Task == nil {
					t.Fatalf("amendment targeted retained completed work: %s", result.Content)
				}
			}
		})
	}
}
