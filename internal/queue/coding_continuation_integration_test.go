package queue

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestIncompleteTaskReturnsToOwnerPlanningAndPrioritizesApprovedContinuation(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Complete the finite approved workload.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "continuation-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim planning: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, job.Instruction)
	operations, ids := continuationTaskOperations("completed", 1)
	later, laterIDs := continuationTaskOperations("untouched", 5)
	operations, ids = append(operations, later...), append(ids, laterIDs...)
	fixture.created, err = repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: fixture.created.LedgerID, OwnerTurnText: "Include these finite tasks.", Mutation: model.LedgerMutation{
		OperationID: "original-tasks", OwnerTurnID: "original-tasks-turn", BaseRevision: fixture.created.Revision,
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids), Operations: operations,
	}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.itemIDs = append(fixture.itemIDs, ids...)
	frozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "continuation-executor")
	if err != nil || executor == nil {
		t.Fatalf("claim execution: %v", err)
	}
	completed, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || completed == nil || completed.ID != "completed" {
		t.Fatalf("first task: %#v %v", completed, err)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *completed)
	selected, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || selected == nil || selected.ID != "task" {
		t.Fatalf("interrupted task: %#v %v", selected, err)
	}
	store := repository.CodingAgentEvidence(executor.Authority)
	questionCall, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "call_opened", Role: codingagent.Execute, TaskID: selected.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "execution_response", TaskID: selected.ID, CallID: questionCall, ResponseKind: codingagent.ExecutionQuestion, Notes: []string{"Which system?"}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.WaitForCodingExecutionInput(ctx, executor.Authority, selected.ID); err != nil {
		t.Fatal(err)
	}
	answerRoot, answerIdentity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "task-clarification", job.ID), JobID: job.ID, ExpectedGeneration: &job.CurrentGeneration, ExpectedWaitingAttempt: &executor.Authority.Attempt, Feedback: "system-a", WorkspaceRoot: answerRoot, WorkspaceIdentity: answerIdentity}); err != nil {
		t.Fatal(err)
	}
	executor, err = repository.ClaimNextStep(ctx, "answered-executor")
	if err != nil || executor == nil {
		t.Fatalf("resume after answer: %#v %v", executor, err)
	}
	accepted, err := repository.CodingAcceptedTaskIDs(ctx, job.ID)
	if err != nil || !reflect.DeepEqual(accepted, []string{"completed"}) {
		t.Fatalf("accepted context: %v %v", accepted, err)
	}
	configuration := codingAgentTestConfiguration()
	controller := codingagent.Controller{JobID: job.ID, Generation: job.CurrentGeneration, Root: "/workspace", Configuration: configuration,
		Environment: continuationTestEnvironment{}, Evidence: repository.CodingAgentEvidence(executor.Authority), CompletedTaskIDs: accepted,
		Providers: map[string]codingagent.Provider{"codex": continuationTestPlanner{t: t}},
	}
	controller.NativeInput = func(ctx context.Context, request codingagent.NativeInputRequest, callID int64, taskID string) (json.RawMessage, error) {
		inputID, err := repository.WaitForCodingNativeInput(ctx, executor.Authority, request, callID, taskID)
		if err != nil {
			return nil, err
		}
		_, err = repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "continuation-planner-native-answer", job.ID), JobID: job.ID, ExpectedGeneration: &job.CurrentGeneration, ExpectedWaitingAttempt: &executor.Authority.Attempt, ExpectedInputID: &inputID, Feedback: "Inspect only the existing scope.", WorkspaceRoot: answerRoot, WorkspaceIdentity: answerIdentity})
		if err != nil {
			return nil, err
		}
		reply, _, err := repository.CodingNativeInputReply(ctx, executor.Authority, inputID)
		return reply, err
	}
	if err := controller.PlanContinuation(ctx, *selected, &codingagent.IncompleteTaskError{Reason: "context exhausted", ThreadID: "original-thread"}, frozen.Ledger); err != nil {
		t.Fatal(err)
	}
	var state string
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT state,current_revision FROM coding_ledgers WHERE id=$1`, frozen.LedgerID).Scan(&state, &revision); err != nil || state != "frozen" || revision != frozen.Revision {
		t.Fatalf("planner mutated authority: %s/%d %v", state, revision, err)
	}
	if err := repository.WaitForCodingExecutionInput(ctx, executor.Authority, selected.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := repository.ClaimNextStep(ctx, "must-wait-for-owner"); err != nil || claim != nil {
		t.Fatalf("unapproved continuation ran: %#v %v", claim, err)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil || details.CodingInputDisposition != model.CodingInputReplan {
		t.Fatalf("public handoff was not routed to planning: %+v %v", details, err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	command := SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "continuation-reply", job.ID), JobID: job.ID,
		ExpectedGeneration: &job.CurrentGeneration, ExpectedWaitingAttempt: &executor.Authority.Attempt,
		Feedback: "Plan the remaining work within the same scope; I will review the ledger.", WorkspaceRoot: root, WorkspaceIdentity: identity}
	replanned, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || replanned.Job.ID != job.ID || replanned.Job.CurrentGeneration != job.CurrentGeneration+1 {
		t.Fatalf("owner replan: %+v %v", replanned, err)
	}
	if replay, err := repository.SubmitJobFeedback(ctx, command); err != nil || replay.Applied {
		t.Fatalf("owner replay: %+v %v", replay, err)
	}
	planClaim, err := repository.ClaimNextStep(ctx, "remaining-work-planner")
	if err != nil || planClaim == nil || planClaim.Step.Action != "v3_coding_plan" {
		t.Fatalf("next step bypassed planning: %#v %v", planClaim, err)
	}
	entries, err := repository.CodingAgentEvidence(planClaim.Authority).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		found = found || entry.Phase == "task_incomplete" && entry.Continuation != nil && entry.Continuation.ThreadID == "original-thread"
	}
	if !found {
		t.Fatal("planning lost the durable incomplete task")
	}
	// More conversation must not erase the pending continuation's priority.
	discussed, err := repository.ProposeUnchangedCodingLedger(ctx, ProposeUnchangedCodingLedgerCommand{LedgerID: frozen.LedgerID, Revision: frozen.Revision, OperationID: "discuss-continuation", OwnerTurnID: "discuss-continuation-turn", OwnerTurnText: command.Feedback})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planClaim.Authority, discussed.LedgerID, discussed.Revision); err != nil {
		t.Fatal(err)
	}
	nextPlanning, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: codingPlanOperationID(t, "more-continuation-discussion", job.ID), JobID: job.ID, Feedback: "Keep discussing only the remaining approved work.", ObservedLedgerRevision: discussed.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity})
	if err != nil {
		t.Fatal(err)
	}
	replanned.Job = nextPlanning.Job
	planClaim, err = repository.ClaimNextStep(ctx, "remaining-work-next-planner")
	if err != nil || planClaim == nil || planClaim.Step.Action != "v3_coding_plan" {
		t.Fatalf("continued planning: %#v %v", planClaim, err)
	}
	remaining, remainingIDs := continuationTaskOperations("remaining", 0)
	proposed, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: frozen.LedgerID, OwnerTurnText: command.Feedback, Mutation: model.LedgerMutation{
		OperationID: "remaining-proposal", OwnerTurnID: "remaining-proposal-turn", BaseRevision: discussed.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn(remainingIDs), Operations: remaining,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if claim, err := repository.ClaimNextStep(ctx, "cannot-execute-proposal"); err != nil || claim != nil {
		t.Fatalf("proposal became execution: %#v %v", claim, err)
	}
	fixture.authority, fixture.created = planClaim.Authority, proposed
	fixture.itemIDs = append(fixture.itemIDs, remainingIDs...)
	refrozen := freezeCodingAgentPlanFixture(t, repository, replanned.Job, fixture)
	resumed, err := repository.ClaimNextStep(ctx, "approved-continuation-executor")
	if err != nil || resumed == nil || resumed.Step.Action != "v3_coding" {
		t.Fatalf("approved execution: %#v %v", resumed, err)
	}
	accepted, err = repository.CodingAcceptedTaskIDs(ctx, job.ID)
	if err != nil || !reflect.DeepEqual(accepted, []string{"completed"}) {
		t.Fatalf("accepted work was reopened: %v %v", accepted, err)
	}
	answers, err := repository.CodingAgentEvidence(resumed.Authority).ReadTaskClarifications(ctx, selected.ID)
	if err != nil || len(answers) != 1 || answers[0].Answer != "system-a" {
		t.Fatalf("continuation lost the prior owner answer: %+v %v", answers, err)
	}
	next, err := repository.NextIncompleteCodingLedgerTask(ctx, resumed.Authority)
	if err != nil || next == nil || next.ID != "remaining" {
		t.Fatalf("continuation lost to lower-ranked untouched work: %#v %v (revision %d)", next, err, refrozen.Revision)
	}
	acceptCodingLedgerTaskFixture(t, repository, resumed.Authority, *next)
	next, err = repository.NextIncompleteCodingLedgerTask(ctx, resumed.Authority)
	if err != nil || next == nil || next.ID != "task" {
		t.Fatalf("interrupted task did not follow its approved continuation: %#v %v", next, err)
	}
}

func continuationTaskOperations(id string, rank int64) ([]model.LedgerMutationOperation, []model.LedgerItemID) {
	task := model.LedgerTask{LedgerItem: repositoryLedgerItem(id, rank), Goal: "Complete " + id, Acceptance: []model.TaskAcceptance{{ID: id + "-acceptance", Text: id + " is verified", Verification: []string{"true"}}}}
	task.ReviewState = model.LedgerReviewProposed
	return []model.LedgerMutationOperation{{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "owner task"}}, []model.LedgerItemID{task.ID}
}

type continuationTestEnvironment struct{}

func (continuationTestEnvironment) Attest(context.Context) error { return nil }

type continuationTestPlanner struct{ t *testing.T }

func (provider continuationTestPlanner) Run(_ context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	provider.t.Helper()
	if call.Role != codingagent.Plan || call.Continuation == nil || !reflect.DeepEqual(call.CompletedTaskIDs, []string{"completed"}) {
		provider.t.Fatalf("incorrect continuation context: %#v", call)
	}
	if len(call.Continuation.Clarifications) != 1 || call.Continuation.Clarifications[0].Answer != "system-a" {
		provider.t.Fatal("planner did not receive the task's durable owner clarification")
	}
	if err := event(codingagent.NativeEvent{Frame: json.RawMessage(`{"fixture":"ordinary-planning"}`)}); err != nil {
		return codingagent.Result{}, err
	}
	input := &codingagent.NativeInput{Request: codingagent.NativeInputRequest{ID: json.RawMessage(`1`), Method: "item/tool/requestUserInput", ThreadID: "fresh-planner", TurnID: "planner-turn", Prompt: "What may I inspect?", Questions: []codingagent.NativeInputQuestion{{ID: "inspect", Text: "What may I inspect?"}}}}
	if err := event(codingagent.NativeEvent{Input: input}); err != nil {
		return codingagent.Result{}, err
	}
	if len(input.Response) == 0 {
		provider.t.Fatal("continuation planner lost its native owner reply")
	}
	return codingagent.Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: "fresh-planner", TurnID: "planner-turn", Completed: true, Content: json.RawMessage(`"The task is incomplete. Review the remaining-work proposal before execution."`)}, nil
}
