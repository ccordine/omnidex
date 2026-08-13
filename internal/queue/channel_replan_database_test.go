package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresChannelReplanRetainsTheSameAuthoritativeJob(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "channel-replan-authority", Scope: model.ChannelScopeUser,
		Name: "Channel replan authority", WorkspaceRoot: "/srv/workspaces/channel-replan-authority",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Keep this exact instruction.")
	if err != nil {
		t.Fatal(err)
	}
	replanned, err := repository.ReplanJob(ctx, testReplanCommand(
		t, job.ID, "channel-same-job", "Use the exact feedback in generation two.",
	))
	if err != nil {
		t.Fatal(err)
	}
	if replanned.ID != job.ID || replanned.CurrentGeneration != 2 {
		t.Fatalf("replanned job=%+v original=%+v", replanned, job)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.ID != job.ID || details.Job.Instruction != job.Instruction ||
		len(details.Steps) != 1 || details.Steps[0].Action != "objective_resolve" ||
		details.Steps[0].Generation != 2 || details.Steps[0].Status != model.StepStatusPending {
		t.Fatalf("replanned channel details=%+v", details)
	}
	var jobCount, userMessageCount int
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs`).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='user'
	`, channel.ID).Scan(&userMessageCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 || userMessageCount != 1 {
		t.Fatalf("replan created successor authority: jobs=%d user_messages=%d", jobCount, userMessageCount)
	}
}

func TestCanonicalReplanStepsRejectsUnboundChatWithoutRelaxingGenericMetadata(t *testing.T) {
	_, pool, ctx := replanTestRepository(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = canonicalReplanStepsTx(ctx, tx, model.Job{
		ID: 41, Pipeline: model.PipelineChat, Instruction: "unbound", Metadata: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("unbound chat was accepted by the replan authority")
	}
	if err := ValidateJobMetadataAuthority(map[string]any{"channel_id": "forged"}); err == nil {
		t.Fatal("generic metadata validation was weakened for channel replan")
	}
}
