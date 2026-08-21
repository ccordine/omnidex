package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func pendingTransitionContextRecords(
	preparation roleplay.SimulationTurnAuthority,
) []queue.ContextSearchRecord {
	if preparation.PendingTransition == nil || len(preparation.PendingTransition.NarrativeEvents) == 0 {
		return nil
	}
	return []queue.ContextSearchRecord{{
		Namespace: "simulation_transition",
		SourceID:  preparation.PendingTransition.OperationID,
		Content:   strings.Join(preparation.PendingTransition.NarrativeEvents, "\n"),
	}}
}

func frozenRoleplayContextRecords(
	responder roleplay.SimulationResponderAuthority,
) ([]queue.ContextSearchRecord, error) {
	projection := responder.NarrativeProjection
	authority := responder.NarrativeAuthority
	if len(authority.MeterKeys) != len(projection.Meters) ||
		len(authority.InventoryItemIDs) != len(projection.Inventory) ||
		len(authority.CanonEventIDs) != len(projection.VisibleFacts) ||
		len(authority.MemoryIDs) != len(projection.Memories) {
		return nil, fmt.Errorf("frozen roleplay content differs from its exact source identities")
	}
	records := []queue.ContextSearchRecord{
		{Namespace: "scene_state", SourceID: authority.SceneID,
			Content: fmt.Sprintf("Current scene: %s. %s", projection.Scene.Title, projection.Scene.Description)},
		{Namespace: "scene_participants", SourceID: authority.SceneID + "-participants",
			Content: "Current participants: " + strings.Join(projection.Participants, ", ") + "."},
	}
	for index, trait := range projection.Viewpoint.Traits {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "character_trait", SourceID: fmt.Sprintf("%s-trait-%d", authority.ViewpointID, index),
			Content: "Character trait: " + trait,
		})
	}
	for index, goal := range projection.Viewpoint.Goals {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "character_goal", SourceID: fmt.Sprintf("%s-goal-%d", authority.ViewpointID, index),
			Content: "Character goal: " + goal,
		})
	}
	for index, meter := range projection.Meters {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "simulation_meter", SourceID: authority.MeterKeys[index],
			Content: fmt.Sprintf("%s is %d on a scale from %d to %d.", meter.Name, meter.Value, meter.Minimum, meter.Maximum),
		})
	}
	for index, item := range projection.Inventory {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "simulation_inventory", SourceID: authority.InventoryItemIDs[index],
			Content: fmt.Sprintf("Inventory item %s: %s (%s).", item.Name, item.Description, item.UseDisplay),
		})
	}
	for index, fact := range projection.VisibleFacts {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "fictional_canon", SourceID: authority.CanonEventIDs[index], Content: fact,
		})
	}
	for index, memory := range projection.Memories {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "character_memory", SourceID: authority.MemoryIDs[index], Content: memory,
		})
	}
	for index, event := range projection.RecentEvents {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "simulation_event", SourceID: fmt.Sprintf("%s-event-%d", authority.SceneID, index),
			Content: event,
		})
	}
	return records, nil
}

func recentRoleplayContextRecords(
	responder roleplay.SimulationResponderAuthority,
) []queue.ContextSearchRecord {
	projection := responder.NarrativeProjection
	authority := responder.NarrativeAuthority
	records := make([]queue.ContextSearchRecord, 0, 6)
	for index := len(projection.RecentEvents) - 1; index >= 0 && len(records) < 2; index-- {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "simulation_event", SourceID: fmt.Sprintf("%s-recent-%d", authority.SceneID, index),
			Content: projection.RecentEvents[index],
		})
	}
	for index := len(projection.VisibleFacts) - 1; index >= 0 && len(records) < 4; index-- {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "fictional_canon", SourceID: authority.CanonEventIDs[index],
			Content: projection.VisibleFacts[index],
		})
	}
	for index := len(projection.Memories) - 1; index >= 0 && len(records) < 6; index-- {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "character_memory", SourceID: authority.MemoryIDs[index],
			Content: projection.Memories[index],
		})
	}
	return records
}
