package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentsActivePlanningMessageQueuesWithoutSupersedingTheLedger(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "conversation-intake-fixture")
	if err != nil || claim == nil {
		t.Fatalf("claim active objective: %v %v", claim, err)
	}
	question := observedSessionQuestion(t, command, initial.Job.ID, initial.Job.CurrentGeneration)
	if result, err := repository.SubmitChannelSessionTurn(ctx, question); err != nil || result.Disposition != model.ChannelSessionTurnQueued {
		t.Fatalf("planning message was not queued: %+v %v", result, err)
	}
	current, err := repository.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || current.Steps[0].ID != claim.Authority.StepID ||
		current.Steps[0].Status != model.StepStatusRunning {
		t.Fatalf("question superseded active attempt: %#v %v", current, err)
	}
	var attempt int64
	if err := pool.QueryRow(ctx, `SELECT current_attempt FROM job_steps WHERE id=$1`, claim.Authority.StepID).Scan(&attempt); err != nil || attempt != claim.Authority.Attempt {
		t.Fatalf("question changed active attempt from %d to %d: %v", claim.Authority.Attempt, attempt, err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(ctx, command.ChannelID, MaxChannelSessionMessages, command.WorkspaceIdentity)
	if err != nil || snapshot.ActiveJob == nil || snapshot.ActiveJob.Job.ID != initial.Job.ID ||
		len(snapshot.Turns) != 2 || snapshot.Turns[1].MessageState != model.ChannelSessionMessagePending {
		t.Fatalf("queued planning message missing from durable history: %#v %v", snapshot, err)
	}
}

func TestActiveSessionQuestionRejectsStaleObservedGeneration(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	question := observedSessionQuestion(t, command, initial.Job.ID, initial.Job.CurrentGeneration+1)
	if _, err := repository.SubmitChannelSessionTurn(ctx, question); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("stale question opened new work: %v", err)
	}
}

func observedSessionQuestion(t *testing.T, command ChannelSessionTurnCommand, jobID, generation int64) ChannelSessionTurnCommand {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"operation_id": codingPlanOperationID(t, "active-conversation-question", jobID),
		"channel_id":   command.ChannelID, "workspace_root": command.WorkspaceRoot,
		"workspace_identity": command.WorkspaceIdentity,
		"text":               "What does the current task do? Answer without changing it.",
		"expected_job_id":    jobID, "expected_generation": generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	var question ChannelSessionTurnCommand
	if err := json.Unmarshal(raw, &question); err != nil {
		t.Fatal(err)
	}
	question.CodingMode = command.CodingMode
	return question
}
