package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingLedgerRepositoryFindingRegressions(t *testing.T) {
	t.Run("out of scope replay and cross command reuse", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Record an excluded request.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		ids := make([]model.LedgerItemID, 0)
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
			ids = append(ids, record.item.ID)
		}
		created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "finding-create", OwnerTurnID: "finding-create-turn",
			OwnerTurnText: "Record an excluded request.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
		})
		if err != nil {
			t.Fatal(err)
		}
		command := ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: "Do not include deployment.",
			Mutation: model.LedgerMutation{
				OperationID: "out-of-scope", BaseRevision: created.Revision, OwnerTurnID: "out-of-scope-turn",
				OwnerTurnDisposition: model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnOutOfScope, Reason: "deployment is a separate request"}}},
			},
		}
		applied, err := repository.ApplyCodingLedgerMutation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := repository.ApplyCodingLedgerMutation(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Applied || replayed.Applied || !reflect.DeepEqual(replayed.Ledger, applied.Ledger) {
			t.Fatalf("out-of-scope apply/replay mismatch: applied=%#v replayed=%#v", applied, replayed)
		}
		if len(applied.OwnerTurnDispositions) != 1 || applied.OwnerTurnDispositions[0].Kind != model.OwnerTurnOutOfScope ||
			applied.OwnerTurnDispositions[0].Reason != "deployment is a separate request" {
			t.Fatalf("out-of-scope disposition is not visible in revision response: %#v", applied.OwnerTurnDispositions)
		}
		if applied.LatestDelta.FromRevision != created.Revision || applied.LatestDelta.ToRevision != applied.Revision ||
			len(applied.LatestDelta.Added)+len(applied.LatestDelta.Amended)+len(applied.LatestDelta.Removed)+len(applied.LatestDelta.Restored) != 0 {
			t.Fatalf("out-of-scope latest delta = %#v", applied.LatestDelta)
		}
		if _, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
			LedgerID: created.LedgerID, Revision: created.Revision,
			OperationID: "out-of-scope", OwnerTurnID: "out-of-scope-turn",
			OwnerTurnText: "Do not include deployment.",
		}); err == nil {
			t.Fatal("a proposed mutation receipt was replayed as a freeze receipt")
		}
	})

	t.Run("large initial replay", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Retain every initial record.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		for index := 0; index < 209; index++ {
			ledger.Tasks = append(ledger.Tasks, model.LedgerTask{
				LedgerItem: repositoryLedgerItem(fmt.Sprintf("constraint-%03d", index), int64(index+10)),
				Goal:       fmt.Sprintf("bounded rule %03d", index),
			})
		}
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
		}
		command := CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "large-create", OwnerTurnID: "large-create-turn",
			OwnerTurnText: "Retain every initial record.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{"task"}),
		}
		created, err := repository.CreateCodingLedger(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := repository.CreateCodingLedger(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		createdCount := len(ledgerRepositoryRecords(created.Ledger))
		replayedCount := len(ledgerRepositoryRecords(replayed.Ledger))
		if createdCount != 210 || replayedCount != createdCount || replayed.Applied {
			t.Fatalf("large initial apply/replay counts = %d/%d applied=%t", createdCount, replayedCount, replayed.Applied)
		}
	})

	t.Run("bounded diff", func(t *testing.T) {
		_, repository := freshCodingPlanRepository(t)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "Page revision differences.", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ledger := repositoryLedgerFixture()
		ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("other", 3), Goal: "other"})
		ids := make([]model.LedgerItemID, 0)
		for _, record := range ledgerRepositoryRecords(ledger) {
			setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
				item.ReviewState = model.LedgerReviewProposed
			})
			ids = append(ids, record.item.ID)
		}
		created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
			JobID: job.ID, OperationID: "diff-create", OwnerTurnID: "diff-create-turn",
			OwnerTurnText: "Page revision differences.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
			OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
		})
		if err != nil {
			t.Fatal(err)
		}
		changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: "Agree two records.",
			Mutation: model.LedgerMutation{
				OperationID: "diff-change", BaseRevision: created.Revision, OwnerTurnID: "diff-change-turn",
				OwnerTurnDisposition: model.RepresentedOwnerTurn(
					[]model.LedgerItemID{"task", "other"}),

				Operations: []model.LedgerMutationOperation{
					{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "agree"},
					{Kind: model.LedgerMutationReview, TargetID: "other", ReviewState: model.LedgerReviewAgreed, Rationale: "agree"},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		diff, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, changed.Revision, CodingLedgerPage{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !diff.HasMore || len(diff.Added)+len(diff.Amended)+len(diff.Removed)+len(diff.Restored) != 1 {
			t.Fatalf("diff page is not bounded: %#v", diff)
		}
	})
}

func TestCodingLedgerInitialJSONReplayNormalizesEmptyCollections(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Replay the exact JSON proposal.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	ledger.Tasks[0].ReadFiles = []string{}
	ledger.Tasks[0].WriteFiles = []string{}
	ledger.Tasks[0].DeleteFiles = []string{}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.EngineeringLedger
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	command := CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "json-replay-create", OwnerTurnID: "json-replay-turn",
		OwnerTurnText: "Replay the exact JSON proposal.", Ledger: decoded,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(

			[]model.LedgerItemID{"task"}),

		Routes: repositoryLedgerRoutes(1),
	}
	created, err := repository.CreateCodingLedger(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.CreateCodingLedger(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Applied || replayed.Applied || replayed.Revision != created.Revision {
		t.Fatalf("JSON proposal apply/replay = %t/%t revisions=%d/%d", created.Applied, replayed.Applied, created.Revision, replayed.Revision)
	}
}

func TestCodingLedgerSearchTreatsLikeMetacharactersLiterally(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Search literal ledger text.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].Goal = "deliver 100% coverage"
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("other", 3), Goal: "deliver 1000 coverage"})
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "literal-search-create", OwnerTurnID: "literal-search-turn",
		OwnerTurnText: "Search literal ledger text.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
		Routes:               repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{
		Revision: created.Revision, Search: "100% coverage",
	})
	if err != nil {
		t.Fatal(err)
	}
	records := ledgerRepositoryRecords(page.Ledger)
	if len(records) != 1 || records[0].item.ID != "task" {
		t.Fatalf("literal wildcard search returned %#v", page.Ledger)
	}
}
