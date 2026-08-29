package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestSimulationUIProjectsGenerationForEveryWorldCharacter(t *testing.T) {
	pool := openIsolatedAPIDatabasePool(t)
	repository := queue.New(pool)
	if err := repository.ResetDatabase(t.Context(), loadAPITestDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID: "roleplay-generation-projection", Scope: model.ChannelScopeUser,
		Name: "Generation projection", WorkspaceRoot: "/srv/workspaces/roleplay-generation-projection",
		Mode: model.ChannelModeRoleplay,
	}, "Signal Archive", "Rin")
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("find roleplay world: found=%t error=%v", found, err)
	}
	if _, err := store.CreateCharacter(t.Context(), world.ID, "Gryph"); err != nil {
		t.Fatal(err)
	}

	projection, err := store.ProjectSimulationUI(
		t.Context(), world.ID, roleplay.SimulationUIPageRequest{Limit: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Characters.Items) != 1 || !projection.Characters.HasMore {
		t.Fatalf("paginated characters=%+v", projection.Characters)
	}
	if len(projection.UserPersonaCharacters) != 2 || len(projection.CharacterGeneration) != 2 {
		t.Fatalf("world characters=%d generation projections=%d", len(projection.UserPersonaCharacters), len(projection.CharacterGeneration))
	}
	for _, character := range projection.UserPersonaCharacters {
		generation, exists := projection.CharacterGeneration[character.ID]
		if !exists || generation.CharacterID != character.ID ||
			generation.Config.LibraryCharacterID != character.LibraryID {
			t.Fatalf("character=%+v generation=%+v exists=%t", character, generation, exists)
		}
		if err := generation.Config.Validate(); err != nil {
			t.Fatalf("character %q generation config: %v", character.ID, err)
		}
	}
}
