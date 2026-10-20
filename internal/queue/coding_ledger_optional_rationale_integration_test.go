package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningLedgerPersistsTasksAndMutationsWithoutInventedRationale(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Build the requested app.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	prerequisite := ledger.Tasks[0]
	prerequisite.ID = "prerequisite"
	prerequisite.Goal = "prepare"
	ledger.Tasks = append(ledger.Tasks, prerequisite)
	ids := []model.LedgerItemID{}
	for _, record := range ledgerRepositoryRecords(ledger) {
		ids = append(ids, record.item.ID)
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) { item.Rationale = ""; item.ReviewState = model.LedgerReviewProposed })
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "plain-plan", OwnerTurnID: "plain-turn", OwnerTurnText: job.Instruction,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids), Ledger: ledger, Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range ledgerRepositoryRecords(created.Ledger) {
		if record.item.Rationale != "" {
			t.Fatalf("invented rationale: %+v", record.item)
		}
	}
	rank := int64(10)
	updated, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Move this task later.", Mutation: model.LedgerMutation{
			OperationID: "plain-mutation", BaseRevision: created.Revision, OwnerTurnID: "plain-mutation-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}),
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank}},
		},
	})
	if err != nil || updated.Revision != created.Revision+1 {
		t.Fatalf("empty mutation rationale rejected: %+v %v", updated, err)
	}
}
