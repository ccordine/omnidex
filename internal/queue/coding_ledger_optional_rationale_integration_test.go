package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningLedgerPersistsItemsEdgesAndMutationsWithoutInventedRationale(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
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
	ledger.Dependencies = []model.LedgerDependency{{LedgerItem: repositoryLedgerItem("dependency", 6), TaskID: "task", PrerequisiteID: "prerequisite", Kind: model.LedgerDependencyRequires}}
	ids := []model.LedgerItemID{}
	for _, record := range ledgerRepositoryRecords(ledger) {
		ids = append(ids, record.item.ID)
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) { item.Rationale = ""; item.ReviewState = model.LedgerReviewProposed })
	}
	ledger.Dependencies[0].Rationale = ""
	ledger.Dependencies[0].ReviewState = model.LedgerReviewProposed
	ids = append(ids, "dependency")
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
	var edges, nonempty int
	if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE rationale<>'') FROM coding_ledger_edges WHERE ledger_id=$1`, created.LedgerID).Scan(&edges, &nonempty); err != nil || edges == 0 || nonempty != 0 {
		t.Fatalf("empty edge rationale not preserved: %d %d %v", edges, nonempty, err)
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
