package roleplay

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

func requirePreparedNarrative(
	expectedProjection NarrativeSimulationProjection,
	expectedAuthority SimulationNarrativeAuthority,
	actualProjection NarrativeSimulationProjection,
	actualAuthority SimulationNarrativeAuthority,
) error {
	if reflect.DeepEqual(actualProjection, expectedProjection) &&
		actualAuthority.Fingerprint == expectedAuthority.Fingerprint {
		return nil
	}
	changes := simulationNarrativeChanges(
		expectedProjection, expectedAuthority, actualProjection, actualAuthority,
	)
	if len(changes) == 0 {
		changes = []string{"projection fingerprint"}
	}
	return fmt.Errorf(
		"%w: prepared narrative changed in %s; restore and retry against the current turn state",
		ErrSimulationStaleRevision, strings.Join(changes, ", "),
	)
}

func simulationNarrativeChanges(
	expectedProjection NarrativeSimulationProjection,
	expectedAuthority SimulationNarrativeAuthority,
	actualProjection NarrativeSimulationProjection,
	actualAuthority SimulationNarrativeAuthority,
) []string {
	changes := make([]string, 0, 8)
	if expectedAuthority.WorldID != actualAuthority.WorldID ||
		expectedAuthority.SceneID != actualAuthority.SceneID ||
		expectedAuthority.SceneRevision != actualAuthority.SceneRevision ||
		expectedProjection.Scene != actualProjection.Scene {
		changes = append(changes, "scene")
	}
	if !slices.Equal(expectedAuthority.ParticipantIDs, actualAuthority.ParticipantIDs) ||
		!slices.Equal(expectedProjection.Participants, actualProjection.Participants) {
		changes = append(changes, "cast")
	}
	if expectedAuthority.ViewpointID != actualAuthority.ViewpointID ||
		!reflect.DeepEqual(expectedProjection.Viewpoint, actualProjection.Viewpoint) {
		changes = append(changes, "responding character")
	}
	if !slices.Equal(
		expectedAuthority.OngoingActionStateIDs, actualAuthority.OngoingActionStateIDs,
	) || !slices.Equal(
		expectedAuthority.OngoingActionCharacterIDs, actualAuthority.OngoingActionCharacterIDs,
	) || !slices.Equal(expectedProjection.OngoingActions, actualProjection.OngoingActions) {
		changes = append(changes, "ongoing actions")
	}
	if !slices.Equal(expectedAuthority.MeterKeys, actualAuthority.MeterKeys) ||
		!reflect.DeepEqual(expectedProjection.Meters, actualProjection.Meters) {
		changes = append(changes, "meters")
	}
	if !slices.Equal(expectedAuthority.InventoryItemIDs, actualAuthority.InventoryItemIDs) ||
		!reflect.DeepEqual(expectedProjection.Inventory, actualProjection.Inventory) {
		changes = append(changes, "inventory")
	}
	if !slices.Equal(expectedAuthority.CanonEventIDs, actualAuthority.CanonEventIDs) ||
		!slices.Equal(expectedProjection.VisibleFacts, actualProjection.VisibleFacts) {
		changes = append(changes, "canon")
	}
	if !slices.Equal(expectedAuthority.MemoryIDs, actualAuthority.MemoryIDs) ||
		!slices.Equal(expectedProjection.Memories, actualProjection.Memories) {
		changes = append(changes, "memories")
	}
	if !slices.Equal(expectedAuthority.TransitionIDs, actualAuthority.TransitionIDs) ||
		!slices.Equal(expectedProjection.RecentEvents, actualProjection.RecentEvents) {
		changes = append(changes, "simulation events")
	}
	return changes
}
