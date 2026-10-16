package codingagent

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningSnapshotOmitsAlreadyCompletedTasksAndDependencies(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: planningLedgerItem("remaining", 5, model.LedgerReviewAgreed), Goal: "remaining work"})
	ledger.Dependencies = []model.LedgerDependency{{LedgerItem: planningLedgerItem("edge", 6, model.LedgerReviewAgreed), TaskID: "remaining", PrerequisiteID: "task", Kind: model.LedgerDependencyRequires}}
	projection := ActivePlanningLedgerContext(&ledger, "task")
	for _, item := range projection.Items {
		if item.ID == "task" || item.Kind == "dependency" || item.ID == "acceptance" || item.ID == "check" {
			t.Fatalf("completed work bloated the active snapshot: %+v", item)
		}
	}
	if len(ledger.Tasks) != 2 || len(ledger.Dependencies) != 1 {
		t.Fatal("prompt projection mutated authoritative state")
	}
}
