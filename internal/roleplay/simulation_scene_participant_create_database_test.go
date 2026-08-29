package roleplay

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateSceneParticipantCommitsCharacterAndSceneMembershipAtomically(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, active := bootstrapRoleplayChannel(t, pool, "scene-persona", "Signal World", "Ada")
	writeTestPersona(t, store, active.ID, "A careful signal reader.")
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Signal Room",
		Description: "Two observers compare a reading.", ParticipantIDs: []string{active.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateSceneParticipant(ctx, world.ID, "Orchid Cartographer")
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.ProjectCurrentScene(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.ProjectSimulationUI(ctx, world.ID, SimulationUIPageRequest{
		Limit: MaxSimulationPageSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	persona, err := store.ProjectPersona(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	narrative, _, err := store.ProjectSimulationNarrative(ctx, world.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 || after.ActiveCharacterID != active.ID ||
		after.Initiative != before.Initiative || len(projection.AllParticipants) != 2 ||
		projection.AllParticipants[1].CharacterID != created.ID ||
		projection.AllParticipants[1].TurnPosition != 1 ||
		persona.Sheet.Summary != created.Name || narrative.Viewpoint.Name != created.Name {
		t.Fatalf("before=%+v after=%+v participants=%+v", before, after, projection.AllParticipants)
	}

	withoutScene, _ := bootstrapRoleplayChannel(t, pool, "scene-persona-none", "Quiet World", "Bex")
	_, err = store.CreateSceneParticipant(ctx, withoutScene.ID, "Must Roll Back")
	if !errors.Is(err, ErrSimulationNotConfigured) {
		t.Fatalf("missing-scene creation error=%v", err)
	}
	characters, err := store.ListCharacters(ctx, withoutScene.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) != 1 || characters[0].Name != "Bex" {
		t.Fatalf("failed atomic creation persisted a character: %+v", characters)
	}
}

func TestCreateSceneParticipantRejectsDuplicateSceneNameAtomically(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, active := bootstrapRoleplayChannel(t, pool, "scene-persona-duplicate", "Signal World", "Ada")
	writeTestPersona(t, store, active.ID, "A careful signal reader.")
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	beforeScene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Signal Room",
		Description: "One observer studies a reading.", ParticipantIDs: []string{active.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeCounts := readSceneParticipantCreationCounts(t, pool, world.ID, sceneID)

	_, err = store.CreateSceneParticipant(ctx, world.ID, active.Name)
	if !errors.Is(err, ErrSimulationConflict) {
		t.Fatalf("duplicate-name creation error=%v", err)
	}
	wantError := fmt.Sprintf("%v: scene participant name %q is ambiguous", ErrSimulationConflict, active.Name)
	if err.Error() != wantError {
		t.Fatalf("duplicate-name creation error=%q want=%q", err, wantError)
	}

	afterScene, err := store.ProjectCurrentScene(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterCounts := readSceneParticipantCreationCounts(t, pool, world.ID, sceneID)
	if afterScene != beforeScene {
		t.Fatalf("duplicate-name creation changed scene: before=%+v after=%+v", beforeScene, afterScene)
	}
	if afterCounts != beforeCounts {
		t.Fatalf("duplicate-name creation committed state: before=%+v after=%+v", beforeCounts, afterCounts)
	}
}

type sceneParticipantCreationCounts struct {
	libraryCharacters int
	worldCharacters   int
	profiles          int
	meters            int
	participants      int
}

func readSceneParticipantCreationCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	worldID, sceneID string,
) sceneParticipantCreationCounts {
	t.Helper()
	var counts sceneParticipantCreationCounts
	err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM roleplay_character_library),
			(SELECT COUNT(*) FROM roleplay_characters WHERE world_id=$1),
			(SELECT COUNT(*) FROM roleplay_character_profiles),
			(SELECT COUNT(*) FROM roleplay_character_meters WHERE world_id=$1),
			(SELECT COUNT(*) FROM roleplay_scene_participants WHERE scene_id=$2)
	`, worldID, sceneID).Scan(
		&counts.libraryCharacters,
		&counts.worldCharacters,
		&counts.profiles,
		&counts.meters,
		&counts.participants,
	)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}
