package roleplay

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestDirectTransitionObservationRemainsBoundToCastAtActionTime(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, mara := bootstrapRoleplayChannel(
		t, pool, "transition-observers", "Observer World", "Mara",
	)
	ivo, err := store.CreateCharacter(ctx, world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, mara.ID, "A careful signal keeper.")
	writeTestPersona(t, store, ivo.ID, "A patient returning observer.")
	if err := store.RegisterMeter(ctx, MeterDefinition{
		WorldID: world.ID, Key: "signal", Name: "Signal",
		Minimum: 0, Maximum: 10, InitialValue: 1,
	}); err != nil {
		t.Fatal(err)
	}
	commandID, err := NewInteractionCommandIdentity()
	if err != nil {
		t.Fatal(err)
	}
	const absentEvent = "The amber beacon records a pulse while Ivo is absent."
	if err := store.RegisterInteractionCommand(ctx, InteractionCommandDefinition{
		ID: commandID, WorldID: world.ID, Key: "pulse", Name: "Pulse",
		Description: absentEvent, ArgumentMode: CommandArgumentNone,
		Effects: []MeterDelta{{MeterKey: "signal", Delta: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Beacon room",
		Description:    "Two observers begin beside an amber beacon.",
		ParticipantIDs: []string{mara.ID, ivo.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	withoutIvo, err := store.UpdateCurrentScene(ctx, SceneUpdate{
		SceneID: scene.ID, WorldID: world.ID, ExpectedRevision: scene.Revision,
		Title: "Beacon room", Description: "Mara remains beside the amber beacon.",
		ParticipantIDs: []string{mara.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	transition := applyTestAction(
		t, store, world.ID, scene.ID, withoutIvo.Revision, "/pulse",
	)
	if _, err := store.UpdateCurrentScene(ctx, SceneUpdate{
		SceneID: scene.ID, WorldID: world.ID, ExpectedRevision: transition.AfterRevision,
		Title: "Beacon room", Description: "Ivo returns after the beacon pulse.",
		ParticipantIDs: []string{mara.ID, ivo.ID},
	}); err != nil {
		t.Fatal(err)
	}

	maraNarrative, maraAuthority, err := store.ProjectSimulationNarrative(
		ctx, world.ID, mara.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	ivoNarrative, ivoAuthority, err := store.ProjectSimulationNarrative(
		ctx, world.ID, ivo.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(maraNarrative.RecentEvents, []string{absentEvent}) ||
		!reflect.DeepEqual(maraAuthority.TransitionIDs, []string{transition.OperationID}) {
		t.Fatalf(
			"present observer narrative/events=%v/%v",
			maraNarrative.RecentEvents, maraAuthority.TransitionIDs,
		)
	}
	if len(ivoNarrative.RecentEvents) != 0 || len(ivoAuthority.TransitionIDs) != 0 {
		t.Fatalf(
			"re-added absent observer received event authority=%v/%v",
			ivoNarrative.RecentEvents, ivoAuthority.TransitionIDs,
		)
	}

	var observerPayload []byte
	if err := pool.QueryRow(ctx, `
		SELECT observer_character_ids
		FROM roleplay_simulation_transitions
		WHERE operation_id=$1
	`, transition.OperationID).Scan(&observerPayload); err != nil {
		t.Fatal(err)
	}
	var observerIDs []string
	if err := json.Unmarshal(observerPayload, &observerIDs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observerIDs, []string{mara.ID}) {
		t.Fatalf("persisted transition observers=%v want [%s]", observerIDs, mara.ID)
	}
	forgedObservers, err := json.Marshal([]string{mara.ID, ivo.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE roleplay_simulation_transitions
		SET observer_character_ids=$2::jsonb
		WHERE operation_id=$1
	`, transition.OperationID, forgedObservers); err == nil {
		t.Fatal("immutable transition accepted a rewritten observer snapshot")
	}
}
