package api

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/modelref"
	"github.com/gryph/omnidex/internal/roleplay"
)

var (
	roleplayCharacterIdentityPattern = regexp.MustCompile(`^rpc_[0-9a-f]{32}$`)
	roleplayLibraryIdentityPattern   = regexp.MustCompile(`^rpl_[0-9a-f]{32}$`)
	roleplayWorldIdentityPattern     = regexp.MustCompile(`^rpw_[0-9a-f]{32}$`)
	roleplaySimulationKeyPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
)

func validateRoleplayCharacterRequest(request roleplayCharacterRequest) error {
	return validateRoleplayText("character name", request.Name, 256, true)
}

func validateRoleplayPersonaRequest(request roleplayPersonaRequest) error {
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 0 {
		return fmt.Errorf("persona expected_revision is required and cannot be negative")
	}
	if err := validateRoleplayText("persona summary", request.Summary, roleplay.MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if err := validateRoleplayText("persona voice", request.Voice, roleplay.MaxSimulationTextBytes, false); err != nil {
		return err
	}
	if err := validateRoleplayTextList("persona traits", request.Traits); err != nil {
		return err
	}
	return validateRoleplayTextList("persona goals", request.Goals)
}

func validateRoleplaySceneRequest(request roleplaySceneRequest, update bool) error {
	if update && (request.ExpectedRevision == nil || *request.ExpectedRevision < 1) {
		return fmt.Errorf("scene update requires a positive expected_revision")
	}
	if !update && request.ExpectedRevision != nil {
		return fmt.Errorf("scene creation cannot carry expected_revision")
	}
	if request.ExpectedDraftRevision != nil && *request.ExpectedDraftRevision < 0 {
		return fmt.Errorf("scene draft expected revision cannot be negative")
	}
	if err := validateRoleplayText("scene title", request.Title, 256, true); err != nil {
		return err
	}
	if err := validateRoleplayText("scene description", request.Description, roleplay.MaxSimulationTextBytes, true); err != nil {
		return err
	}
	if len(request.ParticipantIDs) < 1 || len(request.ParticipantIDs) > roleplay.MaxSceneParticipants {
		return fmt.Errorf("scene requires 1 to %d participants", roleplay.MaxSceneParticipants)
	}
	seen := make(map[string]struct{}, len(request.ParticipantIDs))
	for _, id := range request.ParticipantIDs {
		if !roleplayCharacterIdentityPattern.MatchString(id) {
			return fmt.Errorf("roleplay character identity is invalid")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("scene participant is duplicated")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateRoleplayResponderOrderRequest(request roleplayResponderOrderRequest) error {
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 1 {
		return fmt.Errorf("responder order requires a positive expected_revision")
	}
	if len(request.CharacterIDs) < 1 || len(request.CharacterIDs) > roleplay.MaxSceneParticipants {
		return fmt.Errorf("responder order requires 1 to %d characters", roleplay.MaxSceneParticipants)
	}
	seen := make(map[string]struct{}, len(request.CharacterIDs))
	for _, id := range request.CharacterIDs {
		if !roleplayCharacterIdentityPattern.MatchString(id) {
			return fmt.Errorf("roleplay responder identity is invalid")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("roleplay responder is duplicated")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateRoleplayMeterDefinition(definition roleplay.MeterDefinition) error {
	if !roleplaySimulationKeyPattern.MatchString(definition.Key) {
		return fmt.Errorf("meter key is invalid")
	}
	if err := validateRoleplayText("meter name", definition.Name, 128, true); err != nil {
		return err
	}
	if definition.Minimum < -1_000_000 || definition.Maximum > 1_000_000 || definition.Minimum >= definition.Maximum {
		return fmt.Errorf("meter bounds must be distinct ascending values within -1000000..1000000")
	}
	if definition.InitialValue < definition.Minimum || definition.InitialValue > definition.Maximum {
		return fmt.Errorf("meter initial value must be inside its bounds")
	}
	return nil
}

func validateRoleplayMeterValueRequest(request roleplayMeterValueRequest) error {
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 1 {
		return fmt.Errorf("meter update requires a positive expected_revision")
	}
	if request.Value == nil || *request.Value < -1_000_000 || *request.Value > 1_000_000 {
		return fmt.Errorf("meter value is required within -1000000..1000000")
	}
	return nil
}

func validateRoleplayResearchCapabilityRequest(request roleplayResearchCapabilityRequest) error {
	if request.Enabled == nil {
		return fmt.Errorf("research capability enabled is required")
	}
	return nil
}

func validateRoleplayGenerationRequest(request roleplayGenerationRequest) error {
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 1 {
		return fmt.Errorf("character generation expected_revision is required")
	}
	if request.NarrativeModel != "" {
		if err := modelref.ValidateOllamaName(request.NarrativeModel); err != nil {
			return err
		}
	}
	return nil
}

func validateRoleplayInteractionDefinition(definition roleplay.InteractionCommandDefinition) error {
	if !roleplaySimulationKeyPattern.MatchString(definition.Key) || definition.Key == "give" || definition.Key == "take" {
		return fmt.Errorf("interaction command key is invalid or reserved")
	}
	if err := validateRoleplayText("interaction name", definition.Name, 128, true); err != nil {
		return err
	}
	if err := validateRoleplayText("interaction description", definition.Description, 512, true); err != nil {
		return err
	}
	if definition.ArgumentMode != roleplay.CommandArgumentNone && definition.ArgumentMode != roleplay.CommandArgumentRequired {
		return fmt.Errorf("interaction argument mode is invalid")
	}
	return validateRoleplayEffects(definition.Effects)
}

func validateRoleplayItemDefinition(definition roleplay.ItemTemplateDefinition) error {
	if _, err := roleplay.CanonicalItemAction(roleplay.SimulationActionGive, definition.Name); err != nil {
		return err
	}
	if err := validateRoleplayText("item description", definition.Description, 512, true); err != nil {
		return err
	}
	if (definition.UsePolicy == roleplay.ItemUseFinite && (definition.InitialUses < 1 || definition.InitialUses > 1000)) ||
		(definition.UsePolicy == roleplay.ItemUseInfinite && definition.InitialUses != 0) ||
		(definition.UsePolicy != roleplay.ItemUseFinite && definition.UsePolicy != roleplay.ItemUseInfinite) {
		return fmt.Errorf("item use policy and initial uses are inconsistent")
	}
	if definition.Priority < -1000 || definition.Priority > 1000 {
		return fmt.Errorf("item priority must be within -1000..1000")
	}
	if definition.Trigger != nil {
		if !roleplaySimulationKeyPattern.MatchString(definition.Trigger.MeterKey) ||
			(definition.Trigger.Direction != roleplay.ThresholdAtOrBelow && definition.Trigger.Direction != roleplay.ThresholdAtOrAbove) ||
			definition.Trigger.Threshold < -1_000_000 || definition.Trigger.Threshold > 1_000_000 {
			return fmt.Errorf("item trigger is invalid")
		}
	}
	return validateRoleplayEffects(definition.Effects)
}

func validateRoleplayEffects(effects []roleplay.MeterDelta) error {
	if len(effects) < 1 || len(effects) > roleplay.MaxDefinitionEffects {
		return fmt.Errorf("definition requires 1 to %d meter effects", roleplay.MaxDefinitionEffects)
	}
	seen := make(map[string]struct{}, len(effects))
	for _, effect := range effects {
		if !roleplaySimulationKeyPattern.MatchString(effect.MeterKey) || effect.Delta == 0 ||
			effect.Delta < -100_000 || effect.Delta > 100_000 {
			return fmt.Errorf("meter effect is invalid")
		}
		if _, exists := seen[effect.MeterKey]; exists {
			return fmt.Errorf("meter effect is duplicated")
		}
		seen[effect.MeterKey] = struct{}{}
	}
	return nil
}

func validateRoleplayText(field, value string, maximum int, required bool) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || value != strings.TrimSpace(value) || len([]byte(value)) > maximum {
		return fmt.Errorf("%s must be exact trimmed UTF-8 of at most %d bytes", field, maximum)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func validateRoleplayTextList(field string, values []string) error {
	if len(values) > roleplay.MaxPersonaListEntries {
		return fmt.Errorf("%s exceeds %d entries", field, roleplay.MaxPersonaListEntries)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateRoleplayText(field+" entry", value, 256, true); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains a duplicate", field)
		}
		seen[value] = struct{}{}
	}
	return nil
}
