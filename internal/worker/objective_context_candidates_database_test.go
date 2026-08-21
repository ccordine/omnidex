package worker

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresEmptyContextTermsDoNotAcquireContaminatedAssistantHistory(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "empty-terms-history", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Empty terms history",
		WorkspaceRoot: "/srv/workspaces/empty-terms-history",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, priorJob, err := repository.EnqueueChannelTurn(
		ctx, channel.ID, "IRRELEVANT_BAKERY_OVEN_SENTINEL was recalibrated yesterday.",
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "empty-terms-history-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != priorJob.ID {
		t.Fatalf("claim=%+v want prior job %d", claim, priorJob.ID)
	}
	operationID, err := queue.NewLifecycleOperationID(
		"empty-terms-history-complete", fmt.Sprintf("%d", claim.Step.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, queue.CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: "The unrelated oven record was stored.",
	}); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	contaminated, err := repository.ConversationCandidateAuthorities(ctx, currentJob)
	if err != nil {
		t.Fatal(err)
	}
	if len(contaminated.Turns) == 0 {
		t.Fatal("test setup did not persist prior contaminated history")
	}
	authority, err := newTurnAuthority(currentJob)
	if err != nil {
		t.Fatal(err)
	}
	provider := boundObjectiveContextProvider{
		runtime: &nativeRuntimeV3{svc: &Service{repo: repository}},
		job:     currentJob, authority: authority,
	}
	set, err := provider.Retrieve(ctx, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Required) != 0 || len(set.Optional) != 0 || set.Replan != nil {
		t.Fatalf("empty-term assistant acquisition projected prior history: %#v", set)
	}
}
