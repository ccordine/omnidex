package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func roleplayEvidenceScene(t *testing.T, repository *Repository) (model.Channel, roleplay.World, *roleplay.Store) {
	t.Helper()
	ctx := context.Background()
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: model.ChannelID("roleplay-" + evidenceNonce(t)), Scope: model.ChannelScopeUser,
		Name: "Simulation fixture", Tags: []string{"chat"}, WorkspaceRoot: t.TempDir(), Mode: model.ChannelModeRoleplay,
	}, "Fixture world", "Observer")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world = %#v / %v", world, err)
	}
	characterID := string(channel.RoleplayViewpointCharacterID)
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{CharacterID: characterID,
		Sheet: roleplay.PersonaSheet{Summary: "An observer.", Voice: "Clear and concise.", Traits: []string{}, Goals: []string{}},
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Fixture scene", Description: "An observed setting.", ParticipantIDs: []string{characterID},
	}); err != nil {
		t.Fatal(err)
	}
	return channel, world, store
}
