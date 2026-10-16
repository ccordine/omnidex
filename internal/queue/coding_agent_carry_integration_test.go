package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentsCompletionAcceptsUntouchedCarriedTask(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Complete unchanged accepted work after replanning.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "carried-completion-planner-one")
	if err != nil || planner == nil {
		t.Fatalf("claim first planning step: %#v, %v", planner, err)
	}
	fixture, _ := codingAgentPlanReviewFixture(t, repository, planner, "Complete the exact approved task.")
	firstFrozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "carried-completion-executor-one")
	if err != nil || executor == nil {
		t.Fatalf("claim first execution step: %#v, %v", executor, err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select first task: %#v, %v", task, err)
	}
	acceptCodingLedgerTaskFixture(t, repository, executor.Authority, *task)

	root, identity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "carried-completion-replan", job.ID),
		JobID:       job.ID, Feedback: "Keep the exact accepted task.",
		ObservedLedgerRevision: firstFrozen.Revision,
		WorkspaceRoot:          root, WorkspaceIdentity: identity,
	}); err != nil {
		t.Fatal(err)
	}
	planner, err = repository.ClaimNextStep(ctx, "carried-completion-planner-two")
	if err != nil || planner == nil {
		t.Fatalf("claim replacement planning step: %#v, %v", planner, err)
	}
	proposal, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: firstFrozen.LedgerID, OwnerTurnText: "Keep the exact accepted task.",
		Mutation: model.LedgerMutation{
			OperationID: "carried-completion-confirm", BaseRevision: firstFrozen.Revision,
			OwnerTurnID: "carried-completion-confirm-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{model.LedgerItemID(task.ID)},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: model.LedgerItemID(task.ID),
				ReviewState: model.LedgerReviewAgreed, Rationale: "the exact task remains approved",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, proposal.LedgerID, proposal.Revision); err != nil {
		t.Fatal(err)
	}
	auditID := repositoryReadyPlanningAudit(t, repository, proposal.LedgerID, proposal.Revision, "carried-completion-freeze")
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: proposal.LedgerID, Revision: proposal.Revision,
		OperationID: "carried-completion-freeze", OwnerTurnID: "carried-completion-freeze-turn",
		OwnerTurnText: "Freeze the exact carried task.", AuditID: auditID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, job.ID, frozen.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	executor, err = repository.ClaimNextStep(ctx, "carried-completion-executor-two")
	if err != nil || executor == nil {
		t.Fatalf("claim replacement execution step: %#v, %v", executor, err)
	}
	if selected, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority); err != nil || selected != nil {
		t.Fatalf("carried task was selected again: %#v, %v", selected, err)
	}
	taskCount, checkCount, err := repository.CodingLedgerCompletionCounts(ctx, executor.Authority)
	if err != nil || taskCount != 1 || checkCount != 1 {
		t.Fatalf("carried completion counts = %d/%d, %v", taskCount, checkCount, err)
	}
	operationID, err := NewStepLifecycleOperationID(executor.Authority, LifecycleCompleteStep)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: operationID, Authority: executor.Authority,
		StepID: executor.Step.ID, Output: "All exact frozen tasks are accepted.",
	}); err != nil {
		t.Fatal(err)
	}
	var ledgerState string
	if err := pool.QueryRow(ctx, `SELECT state FROM coding_ledgers WHERE id=$1`, frozen.LedgerID).Scan(&ledgerState); err != nil {
		t.Fatal(err)
	}
	if ledgerState != "completed" {
		t.Fatalf("carried ledger completion state = %q", ledgerState)
	}
}
