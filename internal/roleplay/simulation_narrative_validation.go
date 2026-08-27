package roleplay

import "fmt"

func (projection NarrativeSimulationProjection) Validate() error {
	if projection.Schema != NarrativeSimulationProjectionSchemaV1 {
		return fmt.Errorf("narrative simulation projection schema is invalid")
	}
	if err := validateSimulationText("narrative scene title", projection.Scene.Title, 256, true); err != nil {
		return err
	}
	if err := validateSimulationText("narrative scene description", projection.Scene.Description, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := projection.Scene.Initiative.Validate(); err != nil {
		return fmt.Errorf("narrative scene initiative: %w", err)
	}
	if projection.Participants == nil || len(projection.Participants) < 1 || len(projection.Participants) > MaxSceneParticipants {
		return fmt.Errorf("narrative participants are outside their bound")
	}
	seenParticipants := make(map[string]struct{}, len(projection.Participants))
	for _, name := range projection.Participants {
		if err := validateSimulationText("narrative participant name", name, 256, true); err != nil {
			return err
		}
		if _, duplicate := seenParticipants[name]; duplicate {
			return fmt.Errorf("narrative participant name is duplicated")
		}
		seenParticipants[name] = struct{}{}
	}
	if _, found := seenParticipants[projection.Scene.ActiveCharacterName]; !found {
		return fmt.Errorf("narrative active character is not a participant")
	}
	if _, found := seenParticipants[projection.Viewpoint.Name]; !found {
		return fmt.Errorf("narrative viewpoint is not a participant")
	}
	if err := validateNarrativePersona(projection.Viewpoint); err != nil {
		return err
	}
	seenOngoingCharacters := make(map[string]struct{}, len(projection.OngoingActions))
	if len(projection.OngoingActions) > MaxSceneParticipants {
		return fmt.Errorf("narrative ongoing actions are outside their bound")
	}
	for _, action := range projection.OngoingActions {
		if _, participant := seenParticipants[action.CharacterName]; !participant {
			return fmt.Errorf("narrative ongoing-action character is not a participant")
		}
		if _, duplicate := seenOngoingCharacters[action.CharacterName]; duplicate {
			return fmt.Errorf("narrative ongoing-action character is duplicated")
		}
		seenOngoingCharacters[action.CharacterName] = struct{}{}
		if err := ValidateOngoingActionText(action.Action); err != nil {
			return err
		}
	}
	if projection.Meters == nil || len(projection.Meters) > MaxSimulationMeters {
		return fmt.Errorf("narrative meters are outside their bound")
	}
	for _, meter := range projection.Meters {
		if err := validateSimulationText("narrative meter name", meter.Name, 128, true); err != nil {
			return err
		}
		if meter.Minimum >= meter.Maximum || meter.Value < meter.Minimum || meter.Value > meter.Maximum {
			return fmt.Errorf("narrative meter value is outside its bounds")
		}
	}
	if projection.Inventory == nil || len(projection.Inventory) > MaxInventoryItems {
		return fmt.Errorf("narrative inventory is outside its bound")
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
	return validateNarrativeTextLists(projection)
}

func validateNarrativePersona(persona NarrativePersona) error {
	if err := validateSimulationText("narrative viewpoint name", persona.Name, 256, true); err != nil {
		return err
	}
	return validatePersonaSheet(PersonaSheet{Summary: persona.Summary, Voice: persona.Voice, Traits: persona.Traits, Goals: persona.Goals})
}

func validateNarrativeTextLists(projection NarrativeSimulationProjection) error {
	checks := []struct {
		name    string
		values  []string
		limit   int
		maximum int
	}{
		{"visible facts", projection.VisibleFacts, MaxProjectionEvents, MaxCanonEventBytes},
		{"memories", projection.Memories, MaxProjectionEvents, MaxSimulationTextBytes},
		{"recent events", projection.RecentEvents, MaxSimulationHistory, MaxSimulationTextBytes + 528},
	}
	for _, check := range checks {
		if check.values == nil || len(check.values) > check.limit {
			return fmt.Errorf("narrative %s are outside their bound", check.name)
		}
		for _, value := range check.values {
			if err := validateSimulationText("narrative "+check.name, value, check.maximum, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func CloneNarrativeSimulationProjection(value NarrativeSimulationProjection) NarrativeSimulationProjection {
	value.Participants = cloneSimulationSlice(value.Participants)
	value.OngoingActions = cloneSimulationSlice(value.OngoingActions)
	value.Viewpoint.Traits = cloneSimulationSlice(value.Viewpoint.Traits)
	value.Viewpoint.Goals = cloneSimulationSlice(value.Viewpoint.Goals)
	value.Meters = cloneSimulationSlice(value.Meters)
	value.Inventory = cloneSimulationSlice(value.Inventory)
	value.VisibleFacts = cloneSimulationSlice(value.VisibleFacts)
	value.Memories = cloneSimulationSlice(value.Memories)
	value.RecentEvents = cloneSimulationSlice(value.RecentEvents)
	return value
}

func cloneSimulationSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	clone := make([]T, len(value))
	copy(clone, value)
	return clone
}
