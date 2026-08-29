package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestAssistantConversationCandidatesRejectMutatedCompletedJobInstruction(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "candidate-mutated-assistant-instruction", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeAssistant, Name: "Mutated assistant instruction",
		WorkspaceRoot: "/srv/workspaces/candidate-mutated-assistant-instruction",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, completedJob := completeConversationCandidateExchange(
		t, repository, channel.ID,
		"Remember the exact blue latch instruction.",
		"The exact blue latch instruction is retained.",
	)
	if _, err := repository.pool.Exec(ctx, `
		UPDATE jobs SET instruction='Mutated completed assistant instruction.' WHERE id=$1
	`, completedJob.ID); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := repository.EnqueueChannelTurn(
		ctx, channel.ID, "What was the exact latch instruction?",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConversationCandidateAuthorities(ctx, currentJob); err == nil ||
		!strings.Contains(err.Error(), "unique exact result") {
		t.Fatalf("mutated completed assistant instruction error=%v", err)
	}
}

func TestRoleplayConversationCandidatesRejectMutatedCompletedJobInstruction(t *testing.T) {
	ctx, repository := channelTurnTestRepository(t)
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "candidate-mutated-roleplay-instruction", Scope: model.ChannelScopeUser,
		Mode: model.ChannelModeRoleplay, Name: "Mutated roleplay instruction",
		WorkspaceRoot: "/srv/workspaces/candidate-mutated-roleplay-instruction",
	}, "Instruction Authority", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve roleplay world: found=%t err=%v", found, err)
	}
	viewpointID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, viewpointID)
	_, completedJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "Remember the exact copper bell instruction.",
	)
	if err != nil {
		t.Fatal(err)
	}
	completeRoleplayConversationRound(t, repository, completedJob, map[string]string{
		viewpointID: "Mara nods beside the copper bell.",
	}, "", "")
	if _, err := repository.pool.Exec(ctx, `
		UPDATE jobs SET instruction='Mutated completed roleplay instruction.' WHERE id=$1
	`, completedJob.ID); err != nil {
		t.Fatal(err)
	}
	_, currentJob, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, "What do you remember?",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RoleplayConversationCandidateAuthorities(
		ctx, currentJob, channel.RoleplayViewpointCharacterID,
	); err == nil || !strings.Contains(err.Error(), "instruction differs") {
		t.Fatalf("mutated completed roleplay instruction error=%v", err)
	}
}
