package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestUserIntentCancellationAfterSameJobResume(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Keep accepted progress before cancellation.")
	start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	started, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.InterruptJob(ctx, ReplanJobCommand{OperationID: recoveryOperationID(t), JobID: started.JobID, Feedback: "Pause current work.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	var pausePhase int
	var pauseKind, pauseContext string
	err = pool.QueryRow(ctx, `SELECT phase,kind,context_text FROM channel_conversation_followup_events WHERE job_id=$1 AND text=$2`, started.JobID, "Pause current work.").Scan(&pausePhase, &pauseKind, &pauseContext)
	if err != nil || pausePhase != 1 || pauseKind != "follow_up" || pauseContext != "\n\nuser follow-up:\nPause current work." {
		t.Fatalf("same-generation pause projection: %d %q %q %v", pausePhase, pauseKind, pauseContext, err)
	}
	resume, _ := saveIntent(t, repo, command, started.Revision, "Continue this same job.", statePatch("/job_status", model.JobStatusRunning))
	continued, err := repo.ExecuteUserIntent(ctx, resume.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if continued.JobID != started.JobID || continued.Generation != started.Generation {
		t.Fatal("resume replaced identity")
	}
	var phase int
	var kind, text, contextText string
	err = pool.QueryRow(ctx, `SELECT phase,kind,text,context_text FROM channel_conversation_followup_events WHERE job_id=$1 AND text=$2`, started.JobID, "Continue this same job.").Scan(&phase, &kind, &text, &contextText)
	if err != nil || phase != 1 || kind != "follow_up" || contextText != "\n\nuser follow-up:\n"+text {
		t.Fatalf("resume context projection: %d %q %q %v", phase, kind, contextText, err)
	}
	_, proposal := saveIntent(t, repo, command, continued.Revision, "Cancel this job.", statePatch("/job_status", model.JobStatusCanceled))
	if !proposal.RequiresApproval {
		t.Fatal("cancellation bypassed confirmation")
	}
	confirm, _ := saveIntent(t, repo, command, continued.Revision, "Yes, cancel it.", statePatch("/approval", true))
	receipt, err := repo.ExecuteUserIntent(ctx, confirm.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != model.JobStatusCanceled || receipt.JobID != started.JobID || receipt.Generation != started.Generation {
		t.Fatalf("cancellation changed identity: %+v", receipt)
	}
	replayed, err := repo.ExecuteUserIntent(ctx, confirm.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(replayed, receipt) {
		t.Fatalf("cancellation replay: %+v %v", replayed, err)
	}
}
