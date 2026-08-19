package roleplay

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	minimumMeterValue = -1_000_000
	maximumMeterValue = 1_000_000
	maximumMeterDelta = 100_000
)

func validatePersonaSheet(sheet PersonaSheet) error {
	if err := validateSimulationText("persona summary", sheet.Summary, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := validateSimulationText("persona voice", sheet.Voice, MaxSimulationTextBytes, false); err != nil {
		return err
	}
	if err := validateSimulationTextList("persona traits", sheet.Traits); err != nil {
		return err
	}
	return validateSimulationTextList("persona goals", sheet.Goals)
}

func validateSceneSetup(setup SceneSetup) error {
	if err := validateIdentity(setup.ID, sceneIdentity); err != nil {
		return err
	}
	if err := validateIdentity(setup.WorldID, worldIdentity); err != nil {
		return err
	}
	if err := validateSimulationText("scene title", setup.Title, 256, true); err != nil {
		return err
	}
	if err := validateSimulationText("scene description", setup.Description, MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if len(setup.ParticipantIDs) < 1 || len(setup.ParticipantIDs) > MaxSceneParticipants {
		return fmt.Errorf("scene requires 1 to %d participants", MaxSceneParticipants)
	}
	seen := make(map[string]struct{}, len(setup.ParticipantIDs))
	for _, id := range setup.ParticipantIDs {
		if err := validateIdentity(id, characterIdentity); err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("scene participant %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateMeterDefinition(definition MeterDefinition) error {
	if err := validateIdentity(definition.WorldID, worldIdentity); err != nil {
		return err
	}
	if err := validateSimulationKey("meter", definition.Key); err != nil {
		return err
	}
	if err := validateSimulationText("meter name", definition.Name, 128, true); err != nil {
		return err
	}
	if definition.Minimum < minimumMeterValue || definition.Maximum > maximumMeterValue || definition.Minimum >= definition.Maximum {
		return fmt.Errorf("meter bounds must be distinct ascending values within %d..%d", minimumMeterValue, maximumMeterValue)
	}
	if definition.InitialValue < definition.Minimum || definition.InitialValue > definition.Maximum {
		return fmt.Errorf("meter initial value must be inside its bounds")
	}
	return nil
}

func validateCommandDefinition(definition InteractionCommandDefinition) error {
	if err := validateIdentity(definition.ID, commandIdentity); err != nil {
		return err
	}
	if err := validateIdentity(definition.WorldID, worldIdentity); err != nil {
		return err
	}
	if err := validateSimulationKey("interaction command", definition.Key); err != nil {
		return err
	}
	if definition.Key == "give" || definition.Key == "take" || definition.Key == "research" {
		return fmt.Errorf("interaction command %q is reserved", definition.Key)
	}
	if err := validateSimulationText("interaction command name", definition.Name, 128, true); err != nil {
		return err
	}
	if err := validateSimulationText("interaction command description", definition.Description, 512, true); err != nil {
		return err
	}
	if definition.ArgumentMode != CommandArgumentNone && definition.ArgumentMode != CommandArgumentRequired {
		return fmt.Errorf("interaction command argument mode is invalid")
	}
	return validateMeterDeltas(definition.Effects)
}

func validateItemDefinition(definition ItemTemplateDefinition) error {
	if err := validateIdentity(definition.ID, itemIdentity); err != nil {
		return err
	}
	if err := validateIdentity(definition.WorldID, worldIdentity); err != nil {
		return err
	}
	if _, err := CanonicalItemAction(SimulationActionGive, definition.Name); err != nil {
		return err
	}
	if err := validateSimulationText("item description", definition.Description, 512, true); err != nil {
		return err
	}
	switch definition.UsePolicy {
	case ItemUseFinite:
		if definition.InitialUses < 1 || definition.InitialUses > 1000 {
			return fmt.Errorf("finite item uses must be within 1..1000")
		}
	case ItemUseInfinite:
		if definition.InitialUses != 0 {
			return fmt.Errorf("infinite items must carry zero finite uses")
		}
	default:
		return fmt.Errorf("item use policy must be exactly finite or infinite")
	}
	if definition.Priority < -1000 || definition.Priority > 1000 {
		return fmt.Errorf("item priority must be within -1000..1000")
	}
	if definition.Trigger != nil {
		if err := validateSimulationKey("item trigger meter", definition.Trigger.MeterKey); err != nil {
			return err
		}
		if definition.Trigger.Direction != ThresholdAtOrBelow && definition.Trigger.Direction != ThresholdAtOrAbove {
			return fmt.Errorf("item trigger direction is invalid")
		}
		if definition.Trigger.Threshold < minimumMeterValue || definition.Trigger.Threshold > maximumMeterValue {
			return fmt.Errorf("item trigger threshold is outside supported meter bounds")
		}
	}
	return validateMeterDeltas(definition.Effects)
}

func validateMeterDeltas(effects []MeterDelta) error {
	if len(effects) < 1 || len(effects) > MaxDefinitionEffects {
		return fmt.Errorf("definition requires 1 to %d meter effects", MaxDefinitionEffects)
	}
	seen := make(map[string]struct{}, len(effects))
	for _, effect := range effects {
		if err := validateSimulationKey("meter effect", effect.MeterKey); err != nil {
			return err
		}
		if effect.Delta == 0 || effect.Delta < -maximumMeterDelta || effect.Delta > maximumMeterDelta {
			return fmt.Errorf("meter effect delta must be nonzero within %d..%d", -maximumMeterDelta, maximumMeterDelta)
		}
		if _, duplicate := seen[effect.MeterKey]; duplicate {
			return fmt.Errorf("meter effect %q is duplicated", effect.MeterKey)
		}
		seen[effect.MeterKey] = struct{}{}
	}
	return nil
}

func validateSimulationKey(field, value string) error {
	if !simulationCommandKeyPattern.MatchString(value) {
		return fmt.Errorf("%s key is invalid", field)
	}
	return nil
}

func validateSimulationText(field, value string, maximum int, required bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || value != strings.TrimSpace(value) || len([]byte(value)) > maximum {
		return fmt.Errorf("%s must be exact trimmed UTF-8 of at most %d bytes", field, maximum)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func validateSimulationTextList(field string, values []string) error {
	if len(values) > MaxPersonaListEntries {
		return fmt.Errorf("%s exceeds %d entries", field, MaxPersonaListEntries)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateSimulationText(field+" entry", value, 256, true); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains a duplicate", field)
		}
		seen[value] = struct{}{}
	}
	return nil
}
