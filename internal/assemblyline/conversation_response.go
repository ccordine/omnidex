package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ConversationResponseSchemaV1     = "omnidex.conversation-response.v1"
	maxConversationResponseTextBytes = 8 * 1024
)

type ConversationResponseInput struct {
	Kind             ConversationObjectiveKind `json:"kind"`
	ExactInstruction string                    `json:"exact_instruction"`
	Context          ObjectiveContext          `json:"objective_context"`
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
	verb := "Answer"
	if input.Kind == ObjectiveKindStory {
		verb = "Write the requested narrative response for"
	}
	context, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode objective context: %w", err)
	}
	return strings.Join([]string{
		verb + " exactly one user instruction.",
		"Return only one bounded response leaf. Do not plan, choose capabilities, call tools, manage memory, verify completion, or add objectives.",
		"OBJECTIVE_CONTEXT_JSON:\n" + string(context),
		"EXACT_INSTRUCTION:\n" + input.ExactInstruction,
	}, "\n\n"), nil
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
