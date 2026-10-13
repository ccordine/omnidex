package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestActiveSessionMessageAnswerIsAtomicAndReplayable(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := activeSessionMessageFixture(t, repository)
	command := ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID,
		Kind: assemblyline.ObjectiveKindAnswer, Response: "The current task is applying the requested behavior."}
	result, err := repository.ResolveChannelSessionMessage(ctx, command)
	if err != nil || !result.Applied || result.Replanned || result.ResponseMessage == nil || result.ResponseMessage.Content != command.Response || result.Job.ID != claim.Job.ID || result.Job.CurrentGeneration != claim.Authority.Generation {
		t.Fatalf("answer changed active work or lost its durable reply: %#v %v", result, err)
	}
	if err := repository.RequireActiveStepAttempt(ctx, claim.Authority); err != nil {
		t.Fatalf("answer revoked active attempt: %v", err)
	}
	replay, err := repository.ResolveChannelSessionMessage(ctx, command)
	if err != nil || replay.Applied || replay.ResponseMessage.ID != result.ResponseMessage.ID {
		t.Fatalf("answer replay duplicated its reply: %#v %v", replay, err)
	}
	command.Response = "A different answer."
	if _, err := repository.ResolveChannelSessionMessage(ctx, command); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed replay was accepted: %v", err)
	}
	next, err := repository.NextChannelSessionMessage(ctx, claim.Authority)
	if err != nil || next != nil {
		t.Fatalf("resolved message remains queued: %#v %v", next, err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(ctx, question.ChannelID, MaxChannelSessionMessages, question.WorkspaceIdentity)
	if err != nil || len(snapshot.Transcript.Messages) != 2 || snapshot.Transcript.Messages[1].Content != result.ResponseMessage.Content || snapshot.Turns[1].MessageState != model.ChannelSessionMessageAnswered {
		t.Fatalf("answer is absent from durable history: %#v %v", snapshot, err)
	}
}

func TestActiveSessionMessageReplansOnlyAfterClassification(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := activeSessionMessageFixture(t, repository)
	second := observedSessionQuestion(t, question, claim.Job.ID, claim.Authority.Generation)
	second.OperationID = codingPlanOperationID(t, "second-pending-question", claim.Job.ID)
	if _, err := repository.SubmitChannelSessionTurn(ctx, second); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{
		Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ObjectiveKindWorkspaceMutation,
	})
	if err != nil || !result.Applied || !result.Replanned || result.Job.ID != claim.Job.ID || result.Job.CurrentGeneration != claim.Authority.Generation+1 {
		t.Fatalf("classified mutation did not replan same job: %#v %v", result, err)
	}
	var parentMeaning, otherMeaning string
	if err := pool.QueryRow(ctx, `SELECT payload_json->>'kind' FROM evidence WHERE kind='channel_session_message' AND source_ref=$1`, question.OperationID).Scan(&parentMeaning); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT payload_json->>'kind' FROM evidence WHERE kind='channel_session_message' AND source_ref=$1`, second.OperationID).Scan(&otherMeaning); err != nil {
		t.Fatal(err)
	}
	if parentMeaning != "workspace_mutation" || otherMeaning != "retired" {
		t.Fatalf("redirect retired its own parent or preserved revoked input: parent=%s other=%s", parentMeaning, otherMeaning)
	}
	if _, err := repository.NextChannelSessionMessage(ctx, claim.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("old generation opened another message: %v", err)
	}
	continuity, err := repository.ObjectiveContinuityAuthorities(ctx, result.Job, replanObjectiveBoundary)
	if err != nil || continuity.Replan == nil || continuity.Replan.Feedback != question.Text {
		t.Fatalf("classified redirect lost ordinary continuity: %#v %v", continuity, err)
	}
}

func TestActiveSessionMessageCompletionAndControlBoundaries(t *testing.T) {
	for _, action := range []string{"cancel", "interrupt", "fail"} {
		t.Run(action, func(t *testing.T) {
			pool, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			claim, question := activeSessionMessageFixture(t, repository)
			if err := repository.CompleteStep(ctx, CompleteStepCommand{OperationID: codingPlanOperationID(t, "pending-completion", claim.Job.ID), Authority: claim.Authority, StepID: claim.Step.ID, Output: "workspace_mutation"}); !errors.Is(err, ErrPendingChannelSessionMessage) {
				t.Fatalf("completion dropped pending input: %v", err)
			}
			id := codingPlanOperationID(t, "retire-message-"+action, claim.Job.ID)
			var err error
			switch action {
			case "cancel":
				_, err = repository.CancelJob(ctx, CancelJobCommand{OperationID: id, JobID: claim.Job.ID, Reason: "Stop the fixture.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity})
			case "interrupt":
				_, err = repository.InterruptJob(ctx, ReplanJobCommand{OperationID: id, JobID: claim.Job.ID, Feedback: "Pause the fixture.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity})
			case "fail":
				err = repository.FailStep(ctx, FailStepCommand{OperationID: id, Authority: claim.Authority, StepID: claim.Step.ID, Error: "Expected fixture failure."})
			}
			if err != nil {
				t.Fatal(err)
			}
			var meaning string
			if err := pool.QueryRow(ctx, `SELECT payload_json->>'kind' FROM evidence WHERE kind='channel_session_message' AND source_ref=$1`, question.OperationID).Scan(&meaning); err != nil || meaning != "retired" {
				t.Fatalf("control left accepted input pending: %s %v", meaning, err)
			}
			if _, err := repository.NextChannelSessionMessage(ctx, claim.Authority); !errors.Is(err, ErrStaleStepAttempt) {
				t.Fatalf("revoked authority opened conversation: %v", err)
			}
		})
	}
}

func TestActiveSessionMessageRejectsExpiredLeaseAndIncompleteResolution(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := expiredSessionMessageFixture(t, pool, repository)
	if _, err := pool.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json) VALUES($1,$2,'channel_session_message','lifecycle_operation',$3,'{"kind":"answer"}'::jsonb)`, claim.Job.ID, claim.Step.ID, question.OperationID); err == nil {
		t.Fatal("incomplete terminal evidence bypassed SQL guard")
	}
	if _, err := repository.NextChannelSessionMessage(ctx, claim.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("expired attempt opened inference: %v", err)
	}
	if _, err := repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ObjectiveKindAnswer, Response: "Late answer."}); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("expired attempt committed a reply: %v", err)
	}
}

// This legal initial claim has already elapsed. The schema still owns its
// exact 75-second lease; no expiry update or trigger override is used.
func expiredSessionMessageFixture(t *testing.T, pool *pgxpool.Pool, repository *Repository) (*model.ClaimedStep, ChannelSessionTurnCommand) {
	t.Helper()
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	question := observedSessionQuestion(t, command, initial.Job.ID, initial.Job.CurrentGeneration)
	if _, err := repository.SubmitChannelSessionTurn(ctx, question); err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1`, initial.Job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	const worker = "elapsed-message-fixture"
	if _, err := pool.Exec(ctx, `INSERT INTO job_step_attempts(job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at)
	 VALUES($1,$2,$3,1,$4,clock_timestamp()-interval '76 seconds',clock_timestamp()-interval '76 seconds')`, initial.Job.ID, initial.Job.CurrentGeneration, stepID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_steps SET current_attempt=1,status='running',worker_id=$2,started_at=clock_timestamp() WHERE id=$1`, stepID, worker); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='running' WHERE id=$1`, initial.Job.ID); err != nil {
		t.Fatal(err)
	}
	return &model.ClaimedStep{Job: initial.Job, Step: model.Step{ID: stepID}, Authority: model.StepAttemptAuthority{JobID: initial.Job.ID, Generation: initial.Job.CurrentGeneration, StepID: stepID, Attempt: 1, WorkerID: worker}}, question
}

func TestActiveSessionMessageFailurePreservesWork(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := activeSessionMessageFixture(t, repository)
	command := FailChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Error: "The response provider is unavailable."}
	result, err := repository.FailChannelSessionMessage(ctx, command)
	if err != nil || !result.Applied || result.Replanned || result.ResponseMessage == nil || result.Job.ID != claim.Job.ID || result.Job.CurrentGeneration != claim.Authority.Generation || result.Job.Status != model.JobStatusRunning {
		t.Fatalf("message failure changed original work: %#v %v", result, err)
	}
	if err := repository.RequireActiveStepAttempt(ctx, claim.Authority); err != nil {
		t.Fatalf("message failure revoked work: %v", err)
	}
	replay, err := repository.FailChannelSessionMessage(ctx, command)
	if err != nil || replay.Applied || replay.ResponseMessage.ID != result.ResponseMessage.ID {
		t.Fatalf("message failure was retried: %#v %v", replay, err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(ctx, question.ChannelID, MaxChannelSessionMessages, question.WorkspaceIdentity)
	if err != nil || snapshot.Turns[1].MessageState != model.ChannelSessionMessageFailed || snapshot.Transcript.Messages[1].Content != result.ResponseMessage.Content {
		t.Fatalf("message failure is not visible and durable: %#v %v", snapshot, err)
	}
}

func activeSessionMessageFixture(t *testing.T, repository *Repository) (*model.ClaimedStep, ChannelSessionTurnCommand) {
	t.Helper()
	ctx := context.Background()
	initial, err := repository.SubmitChannelSessionTurn(ctx, channelSessionCodingCommand(t, repository))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "active-message-fixture")
	if err != nil || claim == nil {
		t.Fatalf("claim: %#v %v", claim, err)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(initial.Job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	question := observedSessionQuestion(t, ChannelSessionTurnCommand{ChannelID: binding.ChannelID, WorkspaceRoot: binding.ClientCWD, WorkspaceIdentity: binding.ClientWorkspaceIdentity}, claim.Job.ID, claim.Authority.Generation)
	if _, err := repository.SubmitChannelSessionTurn(ctx, question); err != nil {
		t.Fatal(err)
	}
	return claim, question
}
