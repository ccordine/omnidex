package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestFreezeAndStartCodingLedgerCommitsTogether(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	_, fixture := userIntentFixture(t, repo, "Start the reviewed fixture.")
	agreed, err := repo.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: fixture.created.LedgerID, OwnerTurnText: "Approve this task.", Mutation: model.LedgerMutation{OperationID: "agree", OwnerTurnID: "agree", BaseRevision: fixture.created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed}}}})
	if err != nil {
		t.Fatal(err)
	}
	command := FreezeCodingLedgerCommand{LedgerID: agreed.LedgerID, Revision: agreed.Revision, OperationID: "start", OwnerTurnID: "start", OwnerTurnText: "Start the reviewed fixture."}
	frozen, err := repo.FreezeAndStartCodingLedger(ctx, command)
	if err != nil || !frozen.Applied || frozen.State != "frozen" {
		t.Fatalf("freeze and start: %+v %v", frozen, err)
	}
	replayed, err := repo.FreezeAndStartCodingLedger(ctx, command)
	if err != nil || replayed.Applied || replayed.Revision != frozen.Revision {
		t.Fatalf("start replay: %+v %v", replayed, err)
	}
	claim, err := repo.ClaimNextStep(ctx, "started-work")
	if err != nil || claim == nil || claim.Step.Action != "v3_coding" {
		t.Fatalf("frozen queue did not advance: %+v %v", claim, err)
	}
}

func TestFrozenFailedPlanningResumesApprovedWork(t *testing.T) {
	pool, repo := freshCodingPlanRepository(t)
	ctx := t.Context()
	command := channelSessionCodingCommand(t, repo)
	command.CodingMode = codingagent.Agents
	initial, err := repo.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repo.ClaimNextStep(ctx, "failed-planning-fixture")
	if err != nil || planner == nil {
		t.Fatalf("claim: %v %v", planner, err)
	}
	fixture := codingAgentPlanReviewFixture(t, repo, planner, "Keep this reviewed task and its order.")
	if err := repo.FailStep(ctx, FailStepCommand{OperationID: recoveryOperationID(t), Authority: planner.Authority, StepID: planner.Step.ID, Error: "Concurrent proposal control"}); err != nil {
		t.Fatal(err)
	}
	operations := []model.LedgerMutationOperation{}
	for _, id := range fixture.itemIDs {
		operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReview, TargetID: id, ReviewState: model.LedgerReviewAgreed})
	}
	agreed, err := repo.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: fixture.created.LedgerID, OwnerTurnText: "Approve the retained proposal.", Mutation: model.LedgerMutation{OperationID: "agree", OwnerTurnID: "agree", BaseRevision: fixture.created.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn(fixture.itemIDs), Operations: operations}})
	if err != nil {
		t.Fatal(err)
	}
	freeze := FreezeCodingLedgerCommand{LedgerID: agreed.LedgerID, Revision: agreed.Revision, OperationID: "freeze", OwnerTurnID: "freeze", OwnerTurnText: "Start the approved work."}
	if _, err := repo.FreezeAndStartCodingLedger(ctx, freeze); err == nil {
		t.Fatal("failed planning started through normal waiting boundary")
	}
	retained, _, err := repo.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || retained.State != "proposed" || retained.Revision != agreed.Revision {
		t.Fatalf("failed transition committed freeze: %+v %v", retained, err)
	}
	// Reproduce a frozen receipt committed by the former two-transaction API.
	frozen, err := repo.FreezeCodingLedger(ctx, freeze)
	if err != nil {
		t.Fatal(err)
	}
	resume := ResumeJobCommand{OperationID: recoveryOperationID(t), JobID: initial.Job.ID, ExpectedGeneration: planner.Authority.Generation, ObservedLedgerRevision: frozen.Revision, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity, Feedback: "Continue the exact approved work."}
	for _, invalid := range []string{"workspace", "generation", "revision"} {
		bad := resume
		bad.OperationID = recoveryOperationID(t)
		switch invalid {
		case "workspace":
			bad.WorkspaceIdentity = "foreign-workspace"
		case "generation":
			bad.ExpectedGeneration++
		case "revision":
			bad.ObservedLedgerRevision++
		}
		if _, err := repo.ResumeJob(ctx, bad); err == nil {
			t.Fatalf("accepted stale %s authority", invalid)
		}
	}
	result, err := repo.ResumeJob(ctx, resume)
	if err != nil || !result.Applied || result.Job.CurrentGeneration != planner.Authority.Generation || result.Job.Status != model.JobStatusRunning {
		t.Fatalf("resume: %+v %v", result, err)
	}
	if replay, err := repo.ResumeJob(ctx, resume); err != nil || replay.Applied {
		t.Fatalf("resume replay: %+v %v", replay, err)
	}
	retained, _, err = repo.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || retained.Revision != frozen.Revision || !reflect.DeepEqual(retained.Ledger, frozen.Ledger) {
		t.Fatal("recovery changed approved plan", err)
	}
	var attemptStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM job_step_attempts WHERE step_id=$1 AND attempt=$2`, planner.Step.ID, planner.Authority.Attempt).Scan(&attemptStatus); err != nil || attemptStatus != "failed" {
		t.Fatal("failed attempt history changed", attemptStatus, err)
	}
	execution, err := repo.ClaimNextStep(ctx, "recovered-execution")
	if err != nil || execution == nil || execution.Step.Action != "v3_coding" || execution.Job.ID != initial.Job.ID {
		t.Fatalf("approved work unavailable: %+v %v", execution, err)
	}
	task, err := repo.NextIncompleteCodingLedgerTask(ctx, execution.Authority)
	if err != nil || task == nil {
		t.Fatalf("approved task lost: %+v %v", task, err)
	}
	for task != nil {
		acceptCodingLedgerTaskFixture(t, repo, execution.Authority, *task)
		task, err = repo.NextIncompleteCodingLedgerTask(ctx, execution.Authority)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CompleteStep(ctx, CompleteStepCommand{OperationID: recoveryOperationID(t), Authority: execution.Authority, StepID: execution.Step.ID, Output: "Verified recovered fixture."}); err != nil {
		t.Fatal(err)
	}
	finished, err := repo.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || finished.Job.Status != model.JobStatusCompleted {
		t.Fatalf("recovery stranded at completion: %+v %v", finished.Job, err)
	}
}
