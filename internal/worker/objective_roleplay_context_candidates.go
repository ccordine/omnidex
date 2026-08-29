package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
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

func currentRoundResponseContextRecords(
	preparation roleplay.SimulationTurnAuthority,
	earlier []roleplayRoundResponseAuthority,
) ([]queue.ContextSearchRecord, error) {
	if len(earlier) > len(preparation.Responders) || len(earlier) >= roleplay.MaxSceneParticipants {
		return nil, fmt.Errorf("current roleplay response context exceeds the prepared round")
	}
	records := make([]queue.ContextSearchRecord, len(earlier))
	for index, response := range earlier {
		expected := preparation.Responders[index]
		if response.Position != index || expected.Position != index ||
			string(response.CharacterID) != expected.CharacterID ||
			response.CharacterName != expected.NarrativeProjection.Viewpoint.Name {
			return nil, fmt.Errorf("current roleplay response %d differs from prepared order", index)
		}
		if err := response.CharacterID.Validate(); err != nil {
			return nil, fmt.Errorf("current roleplay response %d character: %w", index, err)
		}
		if strings.TrimSpace(response.CharacterName) == "" {
			return nil, fmt.Errorf("current roleplay response %d has no character name", index)
		}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, response.Text); err != nil {
			return nil, fmt.Errorf("current roleplay response %d: %w", index, err)
		}
		records[index] = queue.ContextSearchRecord{
			Namespace: "current_round_response",
			SourceID: fmt.Sprintf(
				"%s-round-response-%d", response.CharacterID, response.Position,
			),
			Content: fmt.Sprintf(
				"Earlier response by %s in this ordered response round:\n%s",
				response.CharacterName, response.Text,
			),
		}
	}
	return records, nil
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
	if _, err := roleplay.CurrentOngoingActionForCharacter(
		projection, authority, authority.ViewpointID,
	); err != nil {
		return nil, fmt.Errorf("frozen roleplay ongoing-action authority: %w", err)
	}
	if err := projection.Scene.Initiative.Validate(); err != nil {
		return nil, fmt.Errorf("frozen roleplay initiative authority: %w", err)
	}
	records := []queue.ContextSearchRecord{
		{Namespace: "scene_state", SourceID: authority.SceneID,
			Content: fmt.Sprintf(
				"Current scene: %s. %s Active initiative character: %s. Initiative clock: round %d, turn %d, fictional time tick %d.",
				projection.Scene.Title, projection.Scene.Description,
				projection.Scene.ActiveCharacterName, projection.Scene.Initiative.Round,
				projection.Scene.Initiative.Turn, projection.Scene.Initiative.FictionalTimeTick,
			)},
		{Namespace: "scene_participants", SourceID: authority.SceneID + "-participants",
			Content: "Current participants: " + strings.Join(projection.Participants, ", ") + "."},
	}
	for index, action := range projection.OngoingActions {
		records = append(records, queue.ContextSearchRecord{
			Namespace: "ongoing_action", SourceID: authority.OngoingActionStateIDs[index],
			Content: fmt.Sprintf(
				"Current ongoing action for %s: %s", action.CharacterName, action.Action,
			),
		})
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
