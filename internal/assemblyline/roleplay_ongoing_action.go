package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

type RoleplayOngoingActionSource string

const (
	RoleplayOngoingActionSourceAssistantResponse RoleplayOngoingActionSource = "assistant_response"
	RoleplayOngoingActionSourceUserAction        RoleplayOngoingActionSource = "user_action"
)

// RoleplayOngoingActionRelation is a code-owned interpretation of one opaque
// model choice. Its values are never rendered into model context.
type RoleplayOngoingActionRelation string

const (
	RoleplayOngoingActionAbsent      RoleplayOngoingActionRelation = "absent"
	RoleplayOngoingActionUnchanged   RoleplayOngoingActionRelation = "unchanged"
	RoleplayOngoingActionReplacement RoleplayOngoingActionRelation = "replacement"
)

type RoleplayOngoingActionRelationInput struct {
	CharacterName         string                      `json:"character_name"`
	Source                RoleplayOngoingActionSource `json:"source"`
	ExactContribution     string                      `json:"exact_contribution"`
	PreviousOngoingAction *string                     `json:"previous_ongoing_action"`
}

// RoleplayOngoingActionValueInput contains only the context needed to name a
// newly established ongoing action. The previous state remains code-owned.
type RoleplayOngoingActionValueInput struct {
	CharacterName     string                      `json:"character_name"`
	Source            RoleplayOngoingActionSource `json:"source"`
	ExactContribution string                      `json:"exact_contribution"`
}

func NewRoleplayOngoingActionRelationJob(
	input RoleplayOngoingActionRelationInput,
) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkRoleplayOngoingActionRelation, input)
}

func NewRoleplayOngoingActionValueJob(
	input RoleplayOngoingActionValueInput,
) (PortableJob, error) {
	if err := input.validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkRoleplayOngoingActionValue, input)
}

func (input RoleplayOngoingActionRelationInput) validate() error {
	if err := validateRoleplayOngoingActionAuthority(
		input.CharacterName, input.Source, input.ExactContribution,
	); err != nil {
		return err
	}
	if input.PreviousOngoingAction != nil {
		if err := roleplay.ValidateOngoingActionText(*input.PreviousOngoingAction); err != nil {
			return fmt.Errorf("roleplay previous ongoing action: %w", err)
		}
	}
	return nil
}

func (input RoleplayOngoingActionValueInput) validate() error {
	return validateRoleplayOngoingActionAuthority(
		input.CharacterName, input.Source, input.ExactContribution,
	)
}

func validateRoleplayOngoingActionAuthority(
	characterName string,
	source RoleplayOngoingActionSource,
	exactContribution string,
) error {
	if err := validateContextText(
		"roleplay ongoing-action character name", characterName, 256,
	); err != nil {
		return err
	}
	maximum := 0
	switch source {
	case RoleplayOngoingActionSourceAssistantResponse:
		maximum = roleplay.MaxNarrativeResponseBytes
	case RoleplayOngoingActionSourceUserAction:
		maximum = roleplay.MaxUserTurnBytes
	default:
		return fmt.Errorf("roleplay ongoing-action source is invalid")
	}
	return validateGroundedText(
		"roleplay ongoing-action exact contribution", exactContribution,
		maximum, true,
	)
}

func BuildRoleplayOngoingActionRelationPrompt(
	input RoleplayOngoingActionRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := roleplayOngoingActionRelationChoices(input)
	if err != nil {
		return "", err
	}
	context := []string{roleplayOngoingActionContribution(input.CharacterName, input.Source, input.ExactContribution)}
	if input.PreviousOngoingAction == nil {
		context = append([]string{
			input.CharacterName + " had no ongoing action before this contribution.",
		}, context...)
	} else {
		context = append([]string{
			input.CharacterName + "'s ongoing action before this contribution was:\n" + *input.PreviousOngoingAction,
		}, context...)
	}
	return RenderOpaqueModelChoiceQuestion(
		"At the end of this contribution, which description applies to "+input.CharacterName+"'s action?",
		context,
		choices,
	)
}

func DecodeRoleplayOngoingActionRelation(
	input RoleplayOngoingActionRelationInput,
	raw string,
) (RoleplayOngoingActionRelation, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := roleplayOngoingActionRelationChoices(input)
	if err != nil {
		return "", err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return "", err
	}
	relation := RoleplayOngoingActionRelation(value)
	if err := relation.ValidateFor(input); err != nil {
		return "", err
	}
	return relation, nil
}

func (relation RoleplayOngoingActionRelation) ValidateFor(
	input RoleplayOngoingActionRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	switch relation {
	case RoleplayOngoingActionAbsent, RoleplayOngoingActionReplacement:
		return nil
	case RoleplayOngoingActionUnchanged:
		if input.PreviousOngoingAction == nil {
			return fmt.Errorf("an absent previous ongoing action cannot remain unchanged")
		}
		return nil
	default:
		return fmt.Errorf("roleplay ongoing-action relation is not registered")
	}
}

func BuildRoleplayOngoingActionValuePrompt(
	input RoleplayOngoingActionValueInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"What concise present-tense description captures the new action still underway for " + input.CharacterName + " at the end of this contribution?",
		roleplayOngoingActionContribution(input.CharacterName, input.Source, input.ExactContribution),
	}, "\n\n"), nil
}

func DecodeRoleplayOngoingActionValue(
	input RoleplayOngoingActionValueInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	value, err := decodeRawSemanticLeaf(
		"roleplay ongoing action", raw, roleplay.MaxOngoingActionBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := roleplay.ValidateOngoingActionText(value); err != nil {
		return "", err
	}
	return value, nil
}

func roleplayOngoingActionRelationChoices(
	input RoleplayOngoingActionRelationInput,
) ([]OpaqueModelChoice, error) {
	absent, err := NewOpaqueModelChoice(
		"No action is underway.", string(RoleplayOngoingActionAbsent),
	)
	if err != nil {
		return nil, err
	}
	replacement, err := NewOpaqueModelChoice(
		"A new or different action is underway.", string(RoleplayOngoingActionReplacement),
	)
	if err != nil {
		return nil, err
	}
	if input.PreviousOngoingAction == nil {
		return []OpaqueModelChoice{absent, replacement}, nil
	}
	unchanged, err := NewOpaqueModelChoice(
		"The same action remains underway.", string(RoleplayOngoingActionUnchanged),
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{absent, unchanged, replacement}, nil
}

func roleplayOngoingActionContribution(
	characterName string,
	source RoleplayOngoingActionSource,
	exactContribution string,
) string {
	switch source {
	case RoleplayOngoingActionSourceAssistantResponse:
		return characterName + "'s response:\n" + exactContribution
	case RoleplayOngoingActionSourceUserAction:
		return "A user-provided action for " + characterName + ":\n" + exactContribution
	default:
		return ""
	}
}
