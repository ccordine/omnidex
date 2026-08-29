package queue

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayTurnPreflightRejectsStaleAuthorityAndNextTurnUsesCurrentConfiguration(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "fresh-roleplay-turn", Scope: model.ChannelScopeUser, Name: "Fresh roleplay turn",
		WorkspaceRoot: "/srv/workspaces/fresh-roleplay-turn", Mode: model.ChannelModeRoleplay,
	}, "Moonlit Archive", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t err=%v", found, err)
	}
	characterID := string(channel.RoleplayViewpointCharacterID)
	configureRoleplayQueueTestScene(t, store, world.ID, characterID)

	_, firstJob, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, "Mara, answer me.")
	if err != nil {
		t.Fatal(err)
	}
	var firstMetadata channelTurnMetadata
	if err := json.Unmarshal(firstJob.Metadata, &firstMetadata); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{
		CharacterID: characterID, ExpectedRevision: 1,
		Sheet: roleplay.PersonaSheet{
			Summary: "The archive's exact keeper.", Voice: "Warm, concise, and direct.",
			Traits: []string{"attentive"}, Goals: []string{"answer the present turn"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	generation, err := store.ProjectCharacterGeneration(ctx, world.ID, characterID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteCharacterGeneration(ctx, roleplay.CharacterGenerationWriteRequest{
		WorldID: world.ID, CharacterID: characterID, ExpectedRevision: generation.Config.Revision,
		NarrativeModel: "qwen3.5:9b",
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err = repository.ProjectRoleplaySimulationContext(
		ctx, firstMetadata.RoleplaySimulationPreparationID, firstJob.ID,
	)
	staleErr := err
	if !errors.Is(err, roleplay.ErrSimulationStaleRevision) ||
		!strings.Contains(err.Error(), "responding character") ||
		!strings.Contains(err.Error(), "restore and retry") {
		t.Fatalf("stale preflight error=%v", err)
	}

	claim, err := repository.ClaimNextStep(ctx, "stale-roleplay-turn-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != firstJob.ID {
		t.Fatalf("claim=%+v first job=%d", claim, firstJob.ID)
	}
	if err := repository.FailStep(ctx, FailStepCommand{
		OperationID: testLifecycleOperationID(t, "stale-roleplay-preflight", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Error: staleErr.Error(),
	}); err != nil {
		t.Fatal(err)
	}

	_, nextJob, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, "Mara, answer me now.")
	if err != nil {
		t.Fatal(err)
	}
	var nextMetadata channelTurnMetadata
	if err := json.Unmarshal(nextJob.Metadata, &nextMetadata); err != nil {
		t.Fatal(err)
	}
	if nextMetadata.RoleplayGenerationConfig == nil ||
		nextMetadata.RoleplayGenerationConfig.NarrativeModel != "qwen3.5:9b" ||
		nextMetadata.ModelConfig["conversation_response_model"] != "qwen3.5:9b" {
		t.Fatalf("next turn model authority=%+v", nextMetadata)
	}
	_, projection, err := repository.ProjectRoleplaySimulationContext(
		ctx, nextMetadata.RoleplaySimulationPreparationID, nextJob.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Viewpoint.Voice != "Warm, concise, and direct." ||
		projection.Viewpoint.Summary != "The archive's exact keeper." {
		t.Fatalf("next turn projected stale persona=%+v", projection.Viewpoint)
	}
}
