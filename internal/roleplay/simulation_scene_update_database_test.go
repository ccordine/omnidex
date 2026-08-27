package roleplay

import (
	"context"
	"reflect"
	"testing"
)

func TestUpdateCurrentSceneRebasesRemovedInitiativeCursorWithoutAdvancingClock(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, first := bootstrapRoleplayChannel(
		t, pool, "scene-update-rebase", "Rebase world", "First",
	)
	second, err := store.CreateCharacter(ctx, world.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateCharacter(ctx, world.ID, "Third")
	if err != nil {
		t.Fatal(err)
	}
	for _, character := range []Character{first, second, third} {
		writeTestPersona(t, store, character.ID, character.Name+" participant.")
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Original cast",
		Description:    "The initiative character is about to leave.",
		ParticipantIDs: []string{first.ID, second.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := scene.Initiative
	updated, err := store.UpdateCurrentScene(ctx, SceneUpdate{
		SceneID: scene.ID, WorldID: world.ID, ExpectedRevision: scene.Revision,
		Title: "Rebased cast", Description: "The remaining cast keeps the same clock.",
		ParticipantIDs: []string{third.ID, second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ActiveCharacterID != third.ID || updated.Revision != scene.Revision+1 ||
		updated.Initiative != clock {
		t.Fatalf("updated scene=%+v original clock=%+v", updated, clock)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	locked, err := lockSimulationSceneTx(ctx, tx, world.ID, scene.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotParticipants := simulationParticipantIDs(locked.Participants)
	if !reflect.DeepEqual(gotParticipants, []string{third.ID, second.ID}) ||
		locked.Sheet.ActiveCharacterID != third.ID || locked.Sheet.Initiative != clock {
		t.Fatalf("persisted rebase sheet=%+v participants=%v", locked.Sheet, gotParticipants)
	}
}
