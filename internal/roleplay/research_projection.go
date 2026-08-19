package roleplay

import (
	"encoding/json"
	"fmt"
)

const MaxResearchNarrativeProjectionBytes = 24 * 1024

func ValidateResearchNarrativeProjection(projection NarrativeSimulationProjection) error {
	if projection.Schema != NarrativeSimulationProjectionSchemaV1 {
		return fmt.Errorf("roleplay research narrative projection schema is invalid")
	}
	if err := validateSimulationText("narrative scene title", projection.Scene.Title, 256, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative scene description", projection.Scene.Description, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative active character", projection.Scene.ActiveCharacterName, 256, true); err != nil {
		return err
	}
	if len(projection.Participants) < 1 || len(projection.Participants) > MaxSceneParticipants {
		return fmt.Errorf("roleplay research narrative participant count is outside its bound")
	}
	if err := validateNarrativeStrings("narrative participant", projection.Participants, 256, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative viewpoint name", projection.Viewpoint.Name, 256, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative viewpoint summary", projection.Viewpoint.Summary, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative viewpoint voice", projection.Viewpoint.Voice, MaxSimulationTextBytes, false); err != nil {
		return err
	}
	if len(projection.Viewpoint.Traits) > MaxPersonaListEntries || len(projection.Viewpoint.Goals) > MaxPersonaListEntries {
		return fmt.Errorf("roleplay research narrative persona lists exceed their bounds")
	}
	if err := validateNarrativeStrings("narrative trait", projection.Viewpoint.Traits, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := validateNarrativeStrings("narrative goal", projection.Viewpoint.Goals, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if len(projection.Meters) > MaxSimulationMeters || len(projection.Inventory) > MaxInventoryItems ||
		len(projection.VisibleFacts) > MaxProjectionEvents || len(projection.Memories) > MaxProjectionEvents ||
		len(projection.RecentEvents) > MaxSimulationHistory {
		return fmt.Errorf("roleplay research narrative state exceeds a cardinality bound")
	}
	for _, meter := range projection.Meters {
		if err := validateSimulationText("narrative meter name", meter.Name, 128, true); err != nil {
			return err
		}
		if meter.Minimum >= meter.Maximum || meter.Value < meter.Minimum || meter.Value > meter.Maximum {
			return fmt.Errorf("roleplay research narrative meter is outside its bounds")
		}
	}
	for _, item := range projection.Inventory {
		if err := validateSimulationText("narrative item name", item.Name, 256, true); err != nil {
			return err
		}
		if err := validateSimulationText("narrative item description", item.Description, 512, true); err != nil {
			return err
		}
		if err := validateSimulationText("narrative item uses", item.UseDisplay, 64, true); err != nil {
			return err
		}
	}
	for _, values := range []struct {
		label string
		items []string
	}{
		{label: "narrative visible fact", items: projection.VisibleFacts},
		{label: "narrative memory", items: projection.Memories},
		{label: "narrative recent event", items: projection.RecentEvents},
	} {
		if err := validateNarrativeStrings(values.label, values.items, MaxSimulationTextBytes, true); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("encode roleplay research narrative projection: %w", err)
	}
	if len(raw) > MaxResearchNarrativeProjectionBytes {
		return fmt.Errorf("roleplay research narrative projection exceeds %d bytes", MaxResearchNarrativeProjectionBytes)
	}
	return nil
}

func CloneResearchNarrativeProjection(
	projection NarrativeSimulationProjection,
) NarrativeSimulationProjection {
	projection.Participants = append([]string(nil), projection.Participants...)
	projection.Viewpoint.Traits = append([]string(nil), projection.Viewpoint.Traits...)
	projection.Viewpoint.Goals = append([]string(nil), projection.Viewpoint.Goals...)
	projection.Meters = append([]NarrativeMeter(nil), projection.Meters...)
	projection.Inventory = append([]NarrativeInventoryItem(nil), projection.Inventory...)
	projection.VisibleFacts = append([]string(nil), projection.VisibleFacts...)
	projection.Memories = append([]string(nil), projection.Memories...)
	projection.RecentEvents = append([]string(nil), projection.RecentEvents...)
	return projection
}

func validateNarrativeStrings(label string, values []string, maximum int, required bool) error {
	for _, value := range values {
		if err := validateSimulationText(label, value, maximum, required); err != nil {
			return err
		}
	}
	return nil
}
