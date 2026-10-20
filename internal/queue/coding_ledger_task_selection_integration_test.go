package queue

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCodingAgentTaskFromLedgerDetailKeepsOnlySelectedTaskContext(t *testing.T) {
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].Goal += "; deliver the verified implementation"
	detail := CodingLedgerRevision{LedgerID: 1, Revision: 1, Ledger: ledger}
	task, err := codingAgentTaskFromLedgerDetail(detail)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task" || !strings.Contains(task.Goal, "deliver the verified implementation") || !reflect.DeepEqual(task.Acceptance, ledger.Tasks[0].Acceptance) || len(task.ReadFiles)+len(task.WriteFiles)+len(task.DeleteFiles) != 0 {
		t.Fatalf("isolated task context = %#v", task)
	}
}

func TestNextIncompleteCodingLedgerTaskSelectsOneTaskAtATime(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Execute one durable ledger task at a time.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "task-ledger-selector-planner")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim normalized planning: %#v, %v", planner, err)
	}

	ledger := repositoryLedgerFixture()
	ledger.Tasks = append(ledger.Tasks, model.LedgerTask{LedgerItem: repositoryLedgerItem("later-task", 5), Goal: "implement the later task", Acceptance: []model.TaskAcceptance{{ID: "later", Text: "integrated task verified", Verification: []string{"go test ./..."}}}})
	itemIDs := make([]model.LedgerItemID, 0, len(ledgerRepositoryRecords(ledger)))
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
		itemIDs = append(itemIDs, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-selector-create", OwnerTurnID: "task-selector-create-turn",
		OwnerTurnText: "Execute one durable ledger task at a time.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(itemIDs),
		Routes:               repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	reviews := make([]model.LedgerMutationOperation, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		reviews = append(reviews, model.LedgerMutationOperation{
			Kind: model.LedgerMutationReview, TargetID: itemID,
			ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed",
		})
	}
	agreed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree to the exact task ledger.",
		Mutation: model.LedgerMutation{
			OperationID: "task-selector-agree", BaseRevision: created.Revision,
			OwnerTurnID:          "task-selector-agree-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn(itemIDs),
			Operations:           reviews,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, agreed.Revision); err != nil {
		t.Fatal(err)
	}
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: created.LedgerID, Revision: agreed.Revision, OperationID: "task-selector-freeze",
		OwnerTurnID: "task-selector-freeze-turn", OwnerTurnText: "Freeze the task selector fixture.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, job.ID, created.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "task-ledger-selector-executor")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim normalized execution: %#v, %v", executor, err)
	}

	first, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.ID != "task" || first.Goal != "implement" || len(first.Acceptance) != 1 ||
		first.Acceptance[0].ID != "acceptance" || len(first.Acceptance[0].Verification) != 1 || first.Acceptance[0].Verification[0] != "go test ./..." {
		t.Fatalf("first selected task = %#v", first)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *first)
	second, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || second.ID != "later-task" || second.Goal != "implement the later task" {
		t.Fatalf("second selected task = %#v", second)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *second)
	remaining, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || remaining != nil {
		t.Fatalf("remaining task = %#v, %v", remaining, err)
	}
}

func TestCodingLedgerTaskDetailRetainsOwnerContext(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Read task group context.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ledger.Tasks[0].OwnerContext = []model.TaskContext{{Text: "Preserve task context.", Provenance: ledger.Tasks[0].Provenance}}
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) {
			item.ReviewState = model.LedgerReviewProposed
		})
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "task-detail-group-create", OwnerTurnID: "task-detail-group-turn",
		OwnerTurnText: "Read task group context.", Ledger: ledger,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(
			[]model.LedgerItemID{"task"}),

		Routes: repositoryLedgerRoutes(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := repository.CodingLedgerTaskDetail(ctx, created.LedgerID, created.Revision, "task")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detail.Ledger.Tasks[0].OwnerContext, ledger.Tasks[0].OwnerContext) {
		t.Fatal("task context lost", detail)
	}
}
