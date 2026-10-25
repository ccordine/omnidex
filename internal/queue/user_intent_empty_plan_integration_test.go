package queue

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestEmptyPlanStartKeepsDiscussionAndCanBeClarified(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Discuss the requested resource.")
	removed, err := repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: fixture.created.LedgerID, OwnerTurnText: "Remove that task; we still need to discuss the resource.",
		Mutation: model.LedgerMutation{OperationID: "remove-only-task", OwnerTurnID: "remove-only-task", BaseRevision: fixture.created.Revision,
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "Owner removed this task."}},
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	start, _ := saveIntent(t, repo, command, removed.Revision, "Start.", statePatch("/plan_state", "frozen"), statePatch("/job_status", "running"))
	receipt, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || receipt.Status != model.JobStatusWaiting || receipt.Revision != removed.Revision || len(receipt.Patch) != 0 || !strings.Contains(receipt.Summary, "no tasks to start") {
		t.Fatalf("empty start = %+v, %v", receipt, err)
	}
	replay, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	var frozen int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM coding_ledger_revisions WHERE ledger_id=$1 AND state='frozen'`, removed.LedgerID).Scan(&frozen); err != nil || frozen != 0 {
		t.Fatal("empty start froze the ledger", frozen, err)
	}
	reply, _ := saveIntent(t, repo, command, removed.Revision, "Create the local report described in the fixture.", statePatch("/message", map[string]string{"text": "", "recipient": "planner"}))
	continued, err := repo.ExecuteUserIntent(ctx, reply.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || continued.JobID != receipt.JobID || continued.Generation != receipt.Generation+1 {
		t.Fatalf("planning clarification lost job = %+v, %v", continued, err)
	}
}

func TestContentReplyResumesRecoverableJobWithoutCreatingAnotherLedger(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Keep this task and its approved context.")
	start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	started, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "failed-reply")
	if err != nil || claim == nil {
		t.Fatal("missing execution", err)
	}
	if err := repo.FailStep(ctx, FailStepCommand{OperationID: recoveryOperationID(t), StepID: claim.Step.ID, Authority: claim.Authority, Error: "Fixture connection interrupted."}); err != nil {
		t.Fatal(err)
	}
	text := "Use a new disposable environment."
	reply, _ := saveIntent(t, repo, command, started.Revision, text, statePatch("/message", map[string]string{"text": "", "recipient": "codex"}))
	receipt, err := repo.ExecuteUserIntent(ctx, reply.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || receipt.JobID != started.JobID || receipt.Generation != started.Generation || receipt.Revision != started.Revision || receipt.Status != model.JobStatusRunning {
		t.Fatalf("reply forked the task: %+v, %v", receipt, err)
	}
	replay, err := repo.ExecuteUserIntent(ctx, reply.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatalf("reply replay = %+v, %v", replay, err)
	}
	var jobs, ledgers, followups int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM jobs), (SELECT count(*) FROM coding_ledgers), (SELECT count(*) FROM channel_conversation_followup_events WHERE job_id=$1 AND text=$2)`, started.JobID, text).Scan(&jobs, &ledgers, &followups); err != nil || jobs != 1 || ledgers != 1 || followups != 1 {
		t.Fatalf("reply lost context or duplicated work: jobs=%d ledgers=%d followups=%d, %v", jobs, ledgers, followups, err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT status FROM job_steps WHERE id=$1`, claim.Step.ID).Scan(&state); err != nil || state != "failed" {
		t.Fatal("failed history was changed", state, err)
	}
}
