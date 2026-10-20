package queue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCodingLedgerRemoveRestoreKeepsTombstone(t *testing.T) {
	ledger := repositoryLedgerFixture()
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "remove",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].Disposition != model.LedgerDispositionTombstoned || ledger.Tasks[0].ReviewState != "" {
		t.Fatal("remove did not retain a tombstone")
	}
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{
		Kind: model.LedgerMutationRestore, TargetID: "task", Rationale: "restore",
	}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].ID != "task" || ledger.Tasks[0].Disposition != model.LedgerDispositionActive ||
		ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("restore did not preserve identity as a proposed active item")
	}
}

func TestCodingLedgerRejectsRemoveAndRestoreInOneRevision(t *testing.T) {
	tests := []struct {
		name       string
		tombstoned bool
		operations []model.LedgerMutationOperation
	}{
		{
			name: "remove then restore",
			operations: []model.LedgerMutationOperation{
				{Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "remove"},
				{Kind: model.LedgerMutationRestore, TargetID: "task", Rationale: "restore"},
			},
		},
		{
			name:       "restore then remove",
			tombstoned: true,
			operations: []model.LedgerMutationOperation{
				{Kind: model.LedgerMutationRestore, TargetID: "task", Rationale: "restore"},
				{Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "remove"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger := repositoryLedgerFixture()
			if test.tombstoned {
				ledger.Tasks[0].Disposition = model.LedgerDispositionTombstoned
				ledger.Tasks[0].ReviewState = ""
			}
			before, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			err = applyCodingLedgerOperations(&ledger, test.operations)
			if err == nil || !strings.Contains(err.Error(), "removed and restored in one revision") {
				t.Fatalf("remove/restore validation = %v", err)
			}
			after, err := json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("rejected operations mutated ledger\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestCodingLedgerTaskLifecycleUsesRevisionAuthority(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Exercise normalized lifecycle.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}

	routes := []CodingLedgerRoute{
		{Role: "planner", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "executor", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
		{Role: "reviewer", ConfigurationRevision: 1, Provider: "codex", Model: "gpt-5.6-sol", ReasoningEffort: "medium", ServiceTier: "priority"},
	}
	allIDs := []model.LedgerItemID{"task"}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "lifecycle-create", OwnerTurnID: "lifecycle-create-turn",
		OwnerTurnText:        "Exercise normalized lifecycle.",
		OwnerTurnDisposition: model.RepresentedOwnerTurn(allIDs),
		Ledger:               ledger, Routes: routes,
	})
	if err != nil {
		t.Fatal(err)
	}

	apply := func(operationID, turnID string, base int64, operations []model.LedgerMutationOperation, ids []model.LedgerItemID) CodingLedgerRevision {
		t.Helper()
		result, applyErr := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
			LedgerID: created.LedgerID, OwnerTurnText: operationID,
			Mutation: model.LedgerMutation{
				OperationID: operationID, BaseRevision: base, OwnerTurnID: turnID,
				OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
				Operations:           operations,
			},
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		return result
	}

	removedTask := apply("remove-task", "remove-task-turn", created.Revision, []model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "remove task"}}, []model.LedgerItemID{"task"})
	projection, err := repository.ReadCodingLedger(ctx, created.LedgerID, CodingLedgerReadOptions{Revision: removedTask.Revision, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Ledger.Tasks) != 1 || projection.Ledger.Tasks[0].Disposition != model.LedgerDispositionTombstoned {
		t.Fatal("task tombstone lost", projection)
	}
	removedHistory, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, "task", 1)
	if err != nil {
		t.Fatal(err)
	}
	activeHistory, err := repository.CodingLedgerItemHistory(ctx, created.LedgerID, "task", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(removedHistory) != 1 || removedHistory[0].Disposition != model.LedgerDispositionTombstoned ||
		len(activeHistory) != 1 || activeHistory[0].Disposition != model.LedgerDispositionActive {
		t.Fatalf("dependency history pages = %#v / %#v", removedHistory, activeHistory)
	}
	if _, err := repository.CodingLedgerDiff(ctx, created.LedgerID, created.Revision, removedTask.Revision+100); err == nil {
		t.Fatal("diff accepted a revision that does not exist")
	}
}

func TestCodingLedgerUnchangedRemovedTaskRetainsRemovalVersion(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Preserve an optional check tombstone.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("optional-task", 5), Goal: "optional task"})
	ids := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "check-tombstone-create", OwnerTurnID: "check-tombstone-create-turn",
		OwnerTurnText: "Preserve an optional check tombstone.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
		Routes:               repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Remove the optional check.",
		Mutation: model.LedgerMutation{
			OperationID: "remove-optional-task", BaseRevision: created.Revision, OwnerTurnID: "remove-optional-task-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{"optional-task"}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationRemove, TargetID: "optional-task", Rationale: "no longer needed",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rank := int64(20)
	changed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Reorder the task.",
		Mutation: model.LedgerMutation{
			OperationID: "reorder-after-check-removal", BaseRevision: removed.Revision, OwnerTurnID: "reorder-after-check-removal-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(
				[]model.LedgerItemID{"task"}),

			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReorder, TargetID: "task", Rank: &rank, Rationale: "move the task",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var versionCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM coding_ledger_item_versions
		WHERE ledger_id=$1 AND item_id='optional-task'
	`, created.LedgerID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	var itemRevision int64
	var removedByTurnID string
	if err := pool.QueryRow(ctx, `
		SELECT projection.item_revision,version.removed_by_turn_id
		FROM coding_ledger_revision_items projection
		JOIN coding_ledger_item_versions version
		  ON (version.ledger_id,version.item_id,version.revision)=
		     (projection.ledger_id,projection.item_id,projection.item_revision)
		WHERE projection.ledger_id=$1 AND projection.revision=$2
		  AND projection.item_id='optional-task'
	`, created.LedgerID, changed.Revision).Scan(&itemRevision, &removedByTurnID); err != nil {
		t.Fatal(err)
	}
	if versionCount != 2 || itemRevision != removed.Revision || removedByTurnID != "remove-optional-task-turn" {
		t.Fatalf("removed check versions=%d projected_revision=%d removed_by=%q; want 2, %d, remove-optional-task-turn",
			versionCount, itemRevision, removedByTurnID, removed.Revision)
	}
}
