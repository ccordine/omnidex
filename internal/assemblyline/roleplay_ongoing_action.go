package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const RoleplayOngoingStateLeafV1 = "omnidex.roleplay-ongoing-action.v1"

type RoleplayOngoingActionSource string

const (
	RoleplayOngoingActionSourceAssistantResponse RoleplayOngoingActionSource = "assistant_response"
	RoleplayOngoingActionSourceUserAction        RoleplayOngoingActionSource = "user_action"
)

type RoleplayOngoingActionInput struct {
	CharacterName         string                      `json:"character_name"`
	Source                RoleplayOngoingActionSource `json:"source"`
	ExactContribution     string                      `json:"exact_contribution"`
	PreviousOngoingAction *string                     `json:"previous_ongoing_action"`
}

// RoleplayOngoingActionDecision carries exactly one nullable semantic leaf.
// RawMessage preserves the required distinction between an explicit JSON null
// and an omitted field until code validates the candidate.
type RoleplayOngoingActionDecision struct {
	Schema        string          `json:"schema"`
	OngoingAction json.RawMessage `json:"ongoing_action"`
}

func NewRoleplayOngoingActionJob(input RoleplayOngoingActionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkRoleplayOngoingAction, input, input.validate)
}

func (input RoleplayOngoingActionInput) validate() error {
	if err := validateContextText(
		"roleplay ongoing-action character name", input.CharacterName, 256,
	); err != nil {
		return err
	}
	if input.PreviousOngoingAction != nil {
		if err := roleplay.ValidateOngoingActionText(*input.PreviousOngoingAction); err != nil {
			return fmt.Errorf("roleplay previous ongoing action: %w", err)
		}
	}
	maximum := 0
	switch input.Source {
	case RoleplayOngoingActionSourceAssistantResponse:
		maximum = roleplay.MaxNarrativeResponseBytes
	case RoleplayOngoingActionSourceUserAction:
		maximum = roleplay.MaxUserTurnBytes
	default:
		return fmt.Errorf("roleplay ongoing-action source is invalid")
	}
	return validateGroundedText(
		"roleplay ongoing-action exact contribution", input.ExactContribution,
		maximum, true,
	)
}

func (decision RoleplayOngoingActionDecision) ResolveFor(
	input RoleplayOngoingActionInput,
) (*string, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	if decision.Schema != RoleplayOngoingStateLeafV1 {
		return nil, fmt.Errorf(
			"roleplay ongoing-action schema must be %q", RoleplayOngoingStateLeafV1,
		)
	}
	if len(decision.OngoingAction) == 0 {
		return nil, fmt.Errorf("roleplay ongoing_action must be an explicit string or null")
	}
	if bytes.Equal(bytes.TrimSpace(decision.OngoingAction), []byte("null")) {
		return nil, nil
	}
	var action string
	decoder := json.NewDecoder(bytes.NewReader(decision.OngoingAction))
	if err := decoder.Decode(&action); err != nil {
		return nil, fmt.Errorf("roleplay ongoing_action must be an explicit string or null: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("roleplay ongoing_action contains trailing JSON")
		}
		return nil, fmt.Errorf("decode roleplay ongoing_action trailing data: %w", err)
	}
	if err := roleplay.ValidateOngoingActionText(action); err != nil {
		return nil, err
	}
	return &action, nil
}

func (decision RoleplayOngoingActionDecision) ValidateFor(
	input RoleplayOngoingActionInput,
) error {
	_, err := decision.ResolveFor(input)
	return err
}

func DecodeRoleplayOngoingActionDecision(
	input RoleplayOngoingActionInput,
	raw string,
) (RoleplayOngoingActionDecision, error) {
	var decision RoleplayOngoingActionDecision
	if len(raw) > maxPortableCandidateBytes {
		return decision, fmt.Errorf(
			"roleplay ongoing-action candidate exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return decision, fmt.Errorf("decode roleplay ongoing action: %w", err)
	}
	return decision, decision.ValidateFor(input)
}

func BuildRoleplayOngoingActionPrompt(input RoleplayOngoingActionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode roleplay ongoing-action input: %w", err)
	}
	return strings.Join([]string{
		"Determine the single current action, if any, that the named character is still carrying out at the end of this one exact contribution.",
		"The source identifies whether the exact contribution is the character's assistant response or the user's typed action contribution for that character.",
		"The previous_ongoing_action value is the exact current state before the contribution. Preserve it byte-for-byte unless the contribution establishes that the character completed it or replaced it with another action.",
		"Return ongoing_action as one concise standalone present-tense statement naming the character when a replacement action remains underway. Return the exact previous string when it remains underway. Return null only when no action remains underway, including when the contribution establishes completion.",
		"Do not return intentions that have not begun, completed acts, dialogue topics, feelings, traits, facts, explanations, or additional fields.",
		"ROLEPLAY_ONGOING_ACTION_JSON:\n" + string(payload),
	}, "\n\n"), nil
}

func RoleplayOngoingActionResponseSchema() map[string]any {
	return objectSchema([]string{"schema", "ongoing_action"}, map[string]any{
		"schema": map[string]any{
			"type": "string", "const": RoleplayOngoingStateLeafV1,
		},
		"ongoing_action": map[string]any{
			"oneOf": []any{
				map[string]any{
					"type": "string", "minLength": 1,
					"maxLength": roleplay.MaxOngoingActionBytes,
				},
				map[string]any{"type": "null"},
			},
		},
	})
}
