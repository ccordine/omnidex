package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func TestChannelSessionTurnFreezesSavedAgentsOnlyForNewJobs(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	startup := codingAgentTestConfiguration()
	repository.WithCodingAgents(startup)
	ctx := context.Background()
	saved := startup
	saved.Planner.Model = "saved-planner"
	saved.Executor.Model, saved.Executor.Effort = "saved-executor", "medium"
	saved.Reviewer.Model, saved.Reviewer.ServiceTier = "saved-reviewer", "fast"
	if err := repository.StoreCodingAgentConfiguration(ctx, saved); err != nil {
		t.Fatal(err)
	}
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || !result.Applied || result.Disposition != model.ChannelSessionTurnEnqueued {
		t.Fatalf("Agents admission: %#v %v", result, err)
	}
	assertSessionCodingConfiguration(t, result.Job, saved)
	changed := saved
	changed.Planner.Model, changed.Executor.Model, changed.Reviewer.Model = "later-planner", "later-executor", "later-reviewer"
	if err := repository.StoreCodingAgentConfiguration(ctx, changed); err != nil {
		t.Fatal(err)
	}
	replay, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || replay.Applied || replay.Job.ID != result.Job.ID {
		t.Fatalf("Agents replay: %#v %v", replay, err)
	}
	assertSessionCodingConfiguration(t, replay.Job, saved)
	conflicting := command
	conflicting.CodingMode = codingagent.Normal
	if _, err := repository.SubmitChannelSessionTurn(ctx, conflicting); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed-mode replay did not conflict: %v", err)
	}
	// Active controls must use the job's frozen routes even when the latest
	// settings cannot admit a new Agents job.
	if _, err := pool.Exec(ctx, `UPDATE coding_agent_settings SET configuration=jsonb_set(configuration,'{executor,model}','""'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	control := command
	control.OperationID = codingPlanOperationID(t, "session-replan", result.Job.ID)
	control.CodingMode, control.Text = codingagent.Normal, "Refine the approved behavior."
	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: control.OperationID, JobID: result.Job.ID, Feedback: control.Text, WorkspaceRoot: control.WorkspaceRoot, WorkspaceIdentity: control.WorkspaceIdentity})
	if err != nil || !replanned.Applied ||
		replanned.Job.ID != result.Job.ID || replanned.Job.CurrentGeneration != 2 {
		t.Fatalf("frozen-mode replan: %#v %v", replanned, err)
	}
	assertSessionCodingConfiguration(t, replanned.Job, saved)
	if err := repository.StoreCodingAgentConfiguration(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, CancelJobCommand{
		OperationID: codingPlanOperationID(t, "finish-session-fixture", result.Job.ID),
		JobID:       result.Job.ID, Reason: "Fixture complete.",
		WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity,
	}); err != nil {
		t.Fatal(err)
	}
	command.OperationID = codingPlanOperationID(t, "next-session-job", result.Job.ID)
	command.Text = "Apply the next approved behavior."
	next, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || next.Disposition != model.ChannelSessionTurnEnqueued || next.Job.ID == result.Job.ID {
		t.Fatalf("next Agents admission: %#v %v", next, err)
	}
	assertSessionCodingConfiguration(t, next.Job, changed)
}

func TestChannelSessionTurnNormalReplayAndAgentsControlPreserveNormal(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || result.Disposition != model.ChannelSessionTurnEnqueued {
		t.Fatalf("omitted Normal admission: %#v %v", result, err)
	}
	assertSessionCodingConfiguration(t, result.Job, codingagent.Configuration{Mode: codingagent.Normal})
	command.CodingMode = codingagent.Normal
	replay, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || replay.Applied || replay.Job.ID != result.Job.ID {
		t.Fatalf("explicit Normal did not replay omitted Normal: %#v %v", replay, err)
	}
	command.CodingMode = codingagent.Agents
	if _, err := repository.SubmitChannelSessionTurn(ctx, command); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("Agents reused historical Normal operation: %v", err)
	}
	command.OperationID = codingPlanOperationID(t, "normal-session-replan", result.Job.ID)
	command.Text = "Refine the accepted objective."
	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: command.OperationID, JobID: result.Job.ID, Feedback: command.Text, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil || !replanned.Applied || replanned.Job.ID != result.Job.ID {
		t.Fatalf("Agents request changed Normal control: %#v %v", replanned, err)
	}
	assertSessionCodingConfiguration(t, replanned.Job, codingagent.Configuration{Mode: codingagent.Normal})
	command.OperationID = codingPlanOperationID(t, "invalid-session-mode", result.Job.ID)
	command.CodingMode = "unknown"
	if _, err := repository.SubmitChannelSessionTurn(ctx, command); !errors.Is(err, ErrChannelSessionTurnInvalid) {
		t.Fatalf("unknown queue mode was admitted: %v", err)
	}
}

func channelSessionCodingCommand(t *testing.T, repository *Repository) ChannelSessionTurnCommand {
	t.Helper()
	root := t.TempDir()
	directory, err := projectroot.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := projectroot.ClientWorkspaceIdentity("11111111111111111111111111111111", directory)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.EnsureCLIChatSessionChannel(context.Background(), root, identity)
	if err != nil {
		t.Fatal(err)
	}
	return ChannelSessionTurnCommand{
		OperationID: codingPlanOperationID(t, "session-coding-admission", 0),
		ChannelID:   channel.ID, WorkspaceRoot: root, WorkspaceIdentity: identity,
		Text: "Build the approved behavior.",
	}
}

func assertSessionCodingConfiguration(t *testing.T, job model.Job, expected codingagent.Configuration) {
	t.Helper()
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil || configuration != expected {
		t.Fatalf("job %d coding routes: %#v want %#v error=%v", job.ID, configuration, expected, err)
	}
}
