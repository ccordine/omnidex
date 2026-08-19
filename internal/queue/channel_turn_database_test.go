package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresChannelTurnCompletesThroughOneAuthoritativeJob(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "objective-chat", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Objective chat",
		WorkspaceRoot: "/srv/workspaces/objective-chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	project, err := repository.GetProject(ctx, channel.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"model_config":{"conversation_objective_kind_model":"objective-exact","conversation_response_model":"response-exact"}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, channel.ProjectID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	exact := "  Explain the current evidence.  \n"
	message, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, exact)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != exact || job.Instruction != exact || job.Pipeline != model.PipelineChat {
		t.Fatalf("message/job authority was rewritten: message=%q job=%q pipeline=%q", message.Content, job.Instruction, job.Pipeline)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.ProjectID != channel.ProjectID || binding.ClientCWD != channel.WorkspaceRoot {
		t.Fatalf("channel job binding=%+v channel=%+v", binding, channel)
	}
	if binding.ModelConfig.Get("conversation_objective_kind_model") != "objective-exact" ||
		binding.ModelConfig.Get("conversation_response_model") != "response-exact" {
		t.Fatalf("channel job omitted project model snapshot: %+v", binding.ModelConfig)
	}
	var persistedProjectID int64
	if err := repository.pool.QueryRow(ctx, `SELECT project_id FROM jobs WHERE id=$1`, job.ID).Scan(&persistedProjectID); err != nil {
		t.Fatal(err)
	}
	if persistedProjectID != channel.ProjectID {
		t.Fatalf("job project_id=%d want %d", persistedProjectID, channel.ProjectID)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Steps) != 1 || details.Steps[0].Action != "objective_resolve" {
		t.Fatalf("chat steps=%+v", details.Steps)
	}
	claim, err := repository.ClaimNextStep(ctx, "channel-objective-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID || claim.Job.Instruction != exact {
		t.Fatalf("claim=%+v want exact job %d", claim, job.ID)
	}
	output := "Grounded completion."
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "channel-objective-complete", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: output,
	}); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := page.Messages
	if len(messages) != 2 || messages[0].Role != model.ChannelMessageRoleUser || messages[0].Content != exact ||
		messages[1].Role != model.ChannelMessageRoleAssistant || messages[1].Content != output {
		t.Fatalf("channel transcript=%+v", messages)
	}
}

func TestPostgresChannelCompletionRejectsCrossJobAuthorityAtomically(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "cross-job", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant, Name: "Cross job",
		WorkspaceRoot: "/srv/workspaces/cross-job",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := insertChannelMessageForTest(t, repository, channel.ID, model.ChannelMessageRoleUser, "Original authority.")
	if err != nil {
		t.Fatal(err)
	}
	var job model.Job
	err = repository.pool.QueryRow(ctx, `
		INSERT INTO jobs (instruction,pipeline,project_id,status,metadata,current_generation)
		VALUES ('Different authority.','chat',$3,'pending',
			jsonb_build_object('channel_id',$1,'session_id','channel:'||$1,
				'channel_user_message_id',$2,'project_id',$3,'client_cwd',$4,
				'model_config',jsonb_build_object()),1)
		RETURNING id,instruction,pipeline,status,metadata,current_generation,created_at,updated_at
	`, channel.ID, message.ID, channel.ProjectID, channel.WorkspaceRoot).Scan(
		&job.ID, &job.Instruction, &job.Pipeline, &job.Status, &job.Metadata,
		&job.CurrentGeneration, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO job_generations(job_id,generation,purpose) VALUES ($1,1,'initial');
		INSERT INTO job_steps(job_id,action,sort_index,status,generation)
		VALUES ($1,'objective_resolve',5,'pending',1)
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "cross-job-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	err = repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "cross-job-rejected", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: "Must not commit.",
	})
	if err == nil {
		t.Fatal("cross-job channel authority completed")
	}
	page, listErr := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(page.Messages) != 1 || page.Messages[0].Content != "Original authority." {
		t.Fatalf("failed completion changed transcript: %+v", page.Messages)
	}
	details, detailsErr := repository.CurrentJobDetails(ctx, job.ID)
	if detailsErr != nil {
		t.Fatal(detailsErr)
	}
	if details.Job.Status != model.JobStatusRunning || len(details.Steps) != 1 ||
		details.Steps[0].Status != model.StepStatusRunning {
		t.Fatalf("failed completion crossed transaction boundary: %+v", details)
	}
}

func channelTurnTestRepository(t *testing.T) (context.Context, *Repository) {
	t.Helper()
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	return ctx, repository
}
