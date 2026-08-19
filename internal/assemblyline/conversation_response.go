package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	ConversationResponseSchemaV1     = "omnidex.conversation-response.v1"
	maxConversationResponseTextBytes = 8 * 1024
)

type ConversationResponseInput struct {
	Kind             ConversationObjectiveKind               `json:"kind"`
	ExactInstruction string                                  `json:"exact_instruction"`
	Context          ObjectiveContext                        `json:"objective_context"`
	RoleplayContext  *roleplay.NarrativeSimulationProjection `json:"roleplay_context"`
}

type ConversationResponseDecision struct {
	Schema string `json:"schema"`
	Text   string `json:"text"`
}

func NewConversationResponseJob(input ConversationResponseInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkConversationResponse, input, input.validate)
}

func (input ConversationResponseInput) validate() error {
	if input.Kind != ObjectiveKindAnswer && input.Kind != ObjectiveKindStory {
		return fmt.Errorf("conversation response kind %q is unsupported", input.Kind)
	}
	if input.RoleplayContext != nil {
		if input.Kind != ObjectiveKindStory {
			return fmt.Errorf("fictional character authority is valid only for a story response")
		}
		if err := input.RoleplayContext.Validate(); err != nil {
			return fmt.Errorf("roleplay response context: %w", err)
		}
	}
	return (ConversationObjectiveKindInput{
		ExactInstruction: input.ExactInstruction,
		Context:          input.Context,
	}).validate()
}

func (decision ConversationResponseDecision) ValidateFor(input ConversationResponseInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ConversationResponseSchemaV1 {
		return fmt.Errorf("conversation response schema must be %q", ConversationResponseSchemaV1)
	}
	if err := validateGroundedText(
		"conversation response text", decision.Text, maxConversationResponseTextBytes, true,
	); err != nil {
		return err
	}
	return nil
}

func DecodeConversationResponseDecision(
	input ConversationResponseInput,
	raw string,
) (ConversationResponseDecision, error) {
	if len(raw) > maxPortableCandidateBytes {
		return ConversationResponseDecision{}, fmt.Errorf(
			"conversation response candidate exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	var decision ConversationResponseDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return ConversationResponseDecision{}, fmt.Errorf("decode conversation response: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return ConversationResponseDecision{}, err
	}
	return decision, nil
}

func BuildConversationResponsePrompt(input ConversationResponseInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	context, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode objective context: %w", err)
	}
	sections := []string{"Answer exactly one user instruction.",
		"Return one bounded response text leaf that directly satisfies that instruction using only the supplied context.",
		"OBJECTIVE_CONTEXT_JSON:\n" + string(context)}
	if input.RoleplayContext != nil {
		roleplayContext, err := json.Marshal(input.RoleplayContext)
		if err != nil {
			return "", fmt.Errorf("encode roleplay character context: %w", err)
		}
		sections = []string{
			"Write one in-character narrative response to exactly one user turn.",
			"Keep the prose consistent with the supplied fictional reality and already-applied recent events.",
			"FICTIONAL_REALITY_JSON:\n" + string(roleplayContext),
		}
	}
	sections = append(sections, "EXACT_INSTRUCTION:\n"+input.ExactInstruction)
	return strings.Join(sections, "\n\n"), nil
}

func ConversationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "text"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ConversationResponseSchemaV1},
			"text": map[string]any{
				"type": "string", "minLength": 1, "maxLength": maxConversationResponseTextBytes,
			},
		},
	)
}
