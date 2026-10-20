package queue

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func createTaskContractFixture(t *testing.T, ledger model.EngineeringLedger) (*Repository, CodingLedgerRevision) {
	t.Helper()
	_, repo := freshCodingPlanRepository(t)
	job, err := repo.EnqueueCodingJob(t.Context(), "Retain the approved task contract.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ids := []model.LedgerItemID{}
	for i := range ledger.Tasks {
		if ledger.Tasks[i].Disposition == model.LedgerDispositionActive {
			ledger.Tasks[i].ReviewState = model.LedgerReviewProposed
			ids = append(ids, ledger.Tasks[i].ID)
		}
	}
	created, err := repo.CreateCodingLedger(t.Context(), CreateCodingLedgerCommand{JobID: job.ID, OperationID: "task-contract-create", OwnerTurnID: "task-contract-owner", OwnerTurnText: job.Instruction, Ledger: ledger, Routes: repositoryLedgerRoutes(1), OwnerTurnDisposition: model.RepresentedOwnerTurn(ids)})
	if err != nil {
		t.Fatal(err)
	}
	return repo, created
}

// Replaced entities are neither schedulable records nor mutation payloads.
// Exercise real repository admission and inspect the database kind constraint,
// retaining the exact revision and task after every rejected old contract.
func TestCodingLedgerRejectsRemovedEntityContracts(t *testing.T) {
	repo, created := createTaskContractFixture(t, repositoryLedgerFixture())
	var kindConstraint string
	if err := repo.pool.QueryRow(t.Context(), `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='coding_ledger_items'::regclass AND conname='coding_ledger_items_item_kind_check'`).Scan(&kindConstraint); err != nil {
		t.Fatal(err)
	}
	if kindConstraint != "CHECK ((item_kind = 'task'::text))" {
		t.Fatal("database permits non-task ledger records", kindConstraint)
	}
	for _, kind := range []string{"objective", "requirement", "constraint", "acceptance_criterion", "check", "group", "dependency"} {
		t.Run(kind, func(t *testing.T) {
			var item model.LedgerMutationItem
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{%q:{"id":"removed","task_id":"task","prerequisite_id":"other"}}`, kind)), &item); err != nil {
				t.Fatal(err)
			}
			_, err := repo.ApplyCodingLedgerMutation(t.Context(), ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Do not restore removed entities.", Mutation: model.LedgerMutation{OperationID: "removed-" + kind, OwnerTurnID: "removed-owner-" + kind, BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationAdd, Item: &item}}}})
			if err == nil || !strings.Contains(err.Error(), "requires exactly one typed item") {
				t.Fatalf("removed mutation admitted: %v", err)
			}
			current, err := repo.ReadCodingLedger(t.Context(), created.LedgerID, CodingLedgerReadOptions{})
			if err != nil || current.Revision != created.Revision || !reflect.DeepEqual(current.Ledger, created.Ledger) {
				t.Fatal("rejected contract changed authority", current, err)
			}
		})
	}
}

func TestCodingLedgerTaskDetailUsesExactActiveBindings(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0] = restoredPacket(ledger.Tasks[0])
	ledger.Tasks[0].WriteFiles = []string{"fixture.txt"}
	other := model.LedgerTask{LedgerItem: repositoryLedgerItem("other", 5), Goal: "unrelated task", Acceptance: []model.TaskAcceptance{{ID: "private", Text: "unrelated acceptance", Verification: []string{"inspect unrelated task"}}}}
	removed := model.LedgerTask{LedgerItem: repositoryLedgerItem("removed", 6), Goal: "removed task"}
	removed.Disposition, removed.ReviewState = model.LedgerDispositionTombstoned, ""
	ledger.Tasks = append(ledger.Tasks, other, removed)
	repo, created := createTaskContractFixture(t, ledger)
	detail, err := repo.CodingLedgerTaskDetail(t.Context(), created.LedgerID, created.Revision, "task")
	if err != nil || len(detail.Ledger.Tasks) != 1 || !reflect.DeepEqual(detail.Ledger.Tasks[0], created.Ledger.Tasks[0]) {
		t.Fatal("detail changed task-owned context or leaked sibling", detail, err)
	}
	if _, err := repo.CodingLedgerTaskDetail(t.Context(), created.LedgerID, created.Revision, "removed"); err == nil {
		t.Fatal("removed task became active detail")
	}
}

func TestCodingLedgerFrozenProjectionRequiresTaskOwnedVerification(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].Acceptance[0].Verification = nil
	if err := validateFrozenCodingAgentTasks(ledger); err == nil {
		t.Fatal("acceptance without verification entered frozen packet")
	}
	ledger = repositoryLedgerFixture()
	second := model.LedgerTask{LedgerItem: repositoryLedgerItem("second", 3), Goal: "second task", Acceptance: []model.TaskAcceptance{{ID: "acceptance", Text: "second verified", Verification: []string{"inspect second task"}}}}
	ledger.Tasks = append(ledger.Tasks, second)
	if err := validateFrozenCodingAgentTasks(ledger); err != nil {
		t.Fatal("task-local acceptance identity became a global gate", err)
	}
}

// Former dependency amendment regression: the still-applicable protections are
// exact target authority, idempotent replay and int64 rank preservation.
func TestCodingLedgerTaskAmendmentRetainsExactAuthorityAndPrecision(t *testing.T) {
	repo, created := createTaskContractFixture(t, repositoryLedgerFixture())
	base := created.Revision
	for i, rank := range []int64{9007199254740992, 9007199254740993} {
		amended := created.Ledger.Tasks[0]
		amended.Rank = rank
		cmd := ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Move this exact task.", Mutation: model.LedgerMutation{OperationID: fmt.Sprint("precision-", i), OwnerTurnID: fmt.Sprint("precision-owner-", i), BaseRevision: base, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{amended.ID}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: amended.ID, Item: &model.LedgerMutationItem{Task: &amended}}}}}
		changed, err := repo.ApplyCodingLedgerMutation(t.Context(), cmd)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := repo.ApplyCodingLedgerMutation(t.Context(), cmd)
		if err != nil || !changed.Applied || replay.Applied || changed.Revision != replay.Revision || !reflect.DeepEqual(changed.Ledger, replay.Ledger) || len(changed.Ledger.Tasks) != 1 || changed.Ledger.Tasks[0].Rank != rank {
			t.Fatal("exact task amendment/replay failed", changed, replay, err)
		}
		var ids []string
		rows, err := repo.pool.Query(t.Context(), `SELECT item_id FROM coding_ledger_owner_turn_items WHERE ledger_id=$1 AND owner_turn_id=$2 ORDER BY item_id`, created.LedgerID, cmd.Mutation.OwnerTurnID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !reflect.DeepEqual(ids, []string{"task"}) {
			t.Fatal("task amendment changed represented authority", ids, err)
		}
		base = changed.Revision
	}
}

func TestCodingLedgerTaskTombstoneDoesNotResetOtherAgreements(t *testing.T) {
	ledger := repositoryLedgerFixture()
	other := model.LedgerTask{LedgerItem: repositoryLedgerItem("other", 3), Goal: "other task"}
	ledger.Tasks = append(ledger.Tasks, other)
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: other.ID}}); err != nil {
		t.Fatal(err)
	}
	if ledger.Tasks[0].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("removal reset unrelated agreement")
	}
	rank := int64(20)
	before, _ := json.Marshal(ledger)
	if err := applyCodingLedgerOperations(&ledger, []model.LedgerMutationOperation{{Kind: model.LedgerMutationReorder, TargetID: other.ID, Rank: &rank}}); err == nil || !strings.Contains(err.Error(), "is not active") {
		t.Fatal("removed task reordered", err)
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("rejected reorder mutated task state")
	}
}

func TestCodingLedgerTaskPreservesMaximumRationale(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].Rationale = strings.Repeat("r", 8192)
	repo, created := createTaskContractFixture(t, ledger)
	if created.Ledger.Tasks[0].Rationale != ledger.Tasks[0].Rationale {
		t.Fatal("maximum rationale lost")
	}
	current, err := repo.ReadCodingLedger(t.Context(), created.LedgerID, CodingLedgerReadOptions{})
	if err != nil || current.Ledger.Tasks[0].Rationale != ledger.Tasks[0].Rationale {
		t.Fatal("rationale lost on read", err)
	}
	removed, err := repo.ApplyCodingLedgerMutation(t.Context(), ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Remove the task without losing its rationale.", Mutation: model.LedgerMutation{OperationID: "remove-maximum-rationale", OwnerTurnID: "remove-maximum-rationale-owner", BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: "task"}}}})
	if err != nil || len(removed.Ledger.Tasks) != 1 || removed.Ledger.Tasks[0].Disposition != model.LedgerDispositionTombstoned || removed.Ledger.Tasks[0].Rationale != ledger.Tasks[0].Rationale {
		t.Fatal("task tombstone lost maximum rationale", removed, err)
	}
	ledger.Tasks[0].Rationale += "r"
	if err := validateLedgerForRepository(ledger); err == nil {
		t.Fatal("oversize rationale admitted")
	}
}

// Add/remove in one revision must retain the new identity, its tombstone,
// revision diff and history, without reviving a separate dependency entity.
func TestCodingLedgerTransientTaskTombstoneRetainsHistory(t *testing.T) {
	repo, created := createTaskContractFixture(t, repositoryLedgerFixture())
	task := model.LedgerTask{LedgerItem: repositoryLedgerItem("transient", 4), Goal: "temporary task"}
	task.ReviewState = model.LedgerReviewProposed
	changed, err := repo.ApplyCodingLedgerMutation(t.Context(), ApplyCodingLedgerMutationCommand{LedgerID: created.LedgerID, OwnerTurnText: "Add and remove the transient task.", Mutation: model.LedgerMutation{OperationID: "transient-task-change", OwnerTurnID: "transient-task-owner", BaseRevision: created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &task}}, {Kind: model.LedgerMutationRemove, TargetID: task.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Ledger.Tasks) != 2 || changed.Ledger.Tasks[1].ID != task.ID || changed.Ledger.Tasks[1].Disposition != model.LedgerDispositionTombstoned || changed.Ledger.Tasks[1].Rationale != task.Rationale {
		t.Fatal("transient tombstone lost", changed)
	}
	diff, err := repo.CodingLedgerDiff(t.Context(), created.LedgerID, created.Revision, changed.Revision)
	if err != nil || !reflect.DeepEqual(diff.Added, []model.LedgerItemID{task.ID}) {
		t.Fatal("transient identity missing from diff", diff, err)
	}
	history, err := repo.CodingLedgerItemHistory(t.Context(), created.LedgerID, task.ID, 10)
	if err != nil || len(history) != 1 || history[0].Disposition != model.LedgerDispositionTombstoned {
		t.Fatal("transient task history lost", history, err)
	}
}
