package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestUserIntentResumesInterruptedInitialPlanning(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command := channelSessionCodingCommand(t, repo)
	command.CodingMode = codingagent.Agents
	initial, err := repo.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repo.ClaimNextStep(ctx, "planning-recovery")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatal(planner, err)
	}
	paused, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: initial.Job.ID, Feedback: "Pause.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	if paused.Job.CurrentGeneration != initial.Job.CurrentGeneration+1 {
		t.Fatal("missing interrupted boundary")
	}
	resume, _ := saveIntent(t, repo, command, 0, "Continue this job.", statePatch("/job_status", model.JobStatusRunning))
	receipt, err := repo.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.MessageID < 1 || receipt.ControlOperationID == "" {
		t.Fatal("resume lacks exact echo identities", receipt)
	}
	if receipt.JobID != initial.Job.ID || receipt.Generation != paused.Job.CurrentGeneration+1 || receipt.Status != model.JobStatusRunning || receipt.Revision != 0 {
		t.Fatal(receipt)
	}
	replay, err := repo.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatal(replay, err)
	}
	next, err := repo.ClaimNextStep(ctx, "planning-recovery")
	if err != nil || next == nil || next.Step.Action != "v3_coding_plan" || next.Job.ID != initial.Job.ID || next.Job.CurrentGeneration != paused.Job.CurrentGeneration+1 {
		t.Fatal(next, err)
	}
	if !reflect.DeepEqual(next.Job.Metadata, initial.Job.Metadata) {
		t.Fatal("resume changed routing or workspace authority")
	}
	if _, exists, err := repo.CurrentCodingLedgerForJob(ctx, initial.Job.ID); err != nil || exists {
		t.Fatal("resume fabricated approval", exists, err)
	}
}

func TestPlanningResumePreservesLedgerAndApprovalAuthority(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Retain the proposed task.")
	state, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	job := state.LatestJob
	control := ResumeJobCommand{OperationID: recoveryOperationID(t), JobID: job.ID, ExpectedGeneration: job.Generation, ObservedLedgerRevision: fixture.created.Revision, Feedback: "Continue.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
	if _, err := repo.ResumeJob(ctx, control); err == nil {
		t.Fatal("ordinary proposed plan bypassed approval")
	}
	// A normal owner revision reopens planning before the owner interrupts it.
	followup := command
	followup.OperationID, followup.Text = recoveryOperationID(t), "Keep the same task and explain its verification."
	revised, err := repo.SubmitChannelSessionTurn(ctx, followup)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: job.ID, Feedback: "Pause.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	before, exists, err := repo.CurrentCodingLedgerForJob(ctx, job.ID)
	if err != nil || !exists {
		t.Fatal(exists, err)
	}
	if paused.Job.ID != revised.Job.ID {
		t.Fatal("pause replaced job")
	}
	resume, _ := saveIntent(t, repo, command, before.Revision, "Continue this job.", statePatch("/job_status", model.JobStatusRunning))
	result, err := repo.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || result.JobID != job.ID || result.Generation != paused.Job.CurrentGeneration+1 {
		t.Fatal(result, err)
	}
	after, exists, err := repo.CurrentCodingLedgerForJob(ctx, job.ID)
	before.Generation = result.Generation // The ledger read projects the current job generation.
	if err != nil || !exists || !reflect.DeepEqual(before, after) {
		t.Fatal("resume changed retained ledger", err)
	}
	replay, err := repo.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(result, replay) {
		t.Fatal("resume replay", replay, err)
	}
}
