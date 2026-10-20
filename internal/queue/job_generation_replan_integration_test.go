package queue

import (
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestReplanRecoveredCodingStepPreservesHistory(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Retain the approved task and recovery history.")
	start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	if _, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "replan-history-original")
	if err != nil || claim == nil {
		t.Fatalf("claim: %v, %v", claim, err)
	}
	operationID := func() model.LifecycleOperationID {
		id, err := model.NewLifecycleOperationID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	if err := repo.FailStep(ctx, FailStepCommand{OperationID: operationID(), StepID: claim.Step.ID, Authority: claim.Authority, Error: "retained failure"}); err != nil {
		t.Fatal(err)
	}
	var failedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT finished_at FROM job_steps WHERE id=$1`, claim.Step.ID).Scan(&failedAt); err != nil {
		t.Fatal(err)
	}
	before, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResumeJob(ctx, ResumeJobCommand{OperationID: operationID(), JobID: claim.Job.ID, ExpectedGeneration: claim.Job.CurrentGeneration, ObservedLedgerRevision: before.Revision, Feedback: "Resume the same task.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InterruptJob(ctx, ReplanJobCommand{OperationID: operationID(), JobID: claim.Job.ID, Feedback: "Pause for the requested replan.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
		t.Fatal(err)
	}
	replan := ReplanJobCommand{OperationID: operationID(), JobID: claim.Job.ID, ObservedLedgerRevision: before.Revision, Feedback: "Preserve the approved task while replanning.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
	result, err := repo.ReplanJob(ctx, replan)
	if err != nil || !result.Applied || result.Job.ID != claim.Job.ID || result.Job.CurrentGeneration != claim.Job.CurrentGeneration+1 {
		t.Fatalf("replan: %+v, %v", result, err)
	}
	var status, failure string
	var finishedAt time.Time
	var superseded int64
	if err := pool.QueryRow(ctx, `SELECT status,error,finished_at,superseded_at_generation FROM job_steps WHERE id=$1`, claim.Step.ID).Scan(&status, &failure, &finishedAt, &superseded); err != nil {
		t.Fatal(err)
	}
	if status != model.StepStatusFailed || failure != "retained failure" || !finishedAt.Equal(failedAt) || superseded != result.Job.CurrentGeneration {
		t.Fatalf("failed history changed: %s %s %v %d", status, failure, finishedAt, superseded)
	}
	var retired, current int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE superseded_at_generation=$2), count(*) FILTER (WHERE superseded_at_generation IS NULL) FROM job_steps WHERE job_id=$1`, claim.Job.ID, result.Job.CurrentGeneration).Scan(&retired, &current); err != nil {
		t.Fatal(err)
	}
	if retired != 3 || current != 2 {
		t.Fatalf("tail history: retired=%d current=%d", retired, current)
	}
	after, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil || after.LedgerID != before.LedgerID || !reflect.DeepEqual(after.Ledger.Tasks, before.Ledger.Tasks) {
		t.Fatalf("replan lost task context: %+v, %v", after, err)
	}
	if replay, err := repo.ReplanJob(ctx, replan); err != nil || replay.Applied || replay.Job.CurrentGeneration != result.Job.CurrentGeneration {
		t.Fatalf("replan replay: %+v, %v", replay, err)
	}
}
