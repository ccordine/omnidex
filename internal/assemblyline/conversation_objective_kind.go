package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

const (
	ConversationObjectiveKindSchemaV1 = "omnidex.conversation-objective-kind.v1"
	maxConversationInstructionBytes   = model.MaxFreeFormTurnBytes
)

type ConversationObjectiveKind string

const (
	ObjectiveKindAnswer            ConversationObjectiveKind = "answer"
	ObjectiveKindRepositoryRead    ConversationObjectiveKind = "repository_read"
	ObjectiveKindWorkspaceMutation ConversationObjectiveKind = "workspace_mutation"
	ObjectiveKindExternalAnswer    ConversationObjectiveKind = "external_answer"
	ObjectiveKindStory             ConversationObjectiveKind = "story"
)

type ConversationObjectiveKindInput struct {
	ExactInstruction string           `json:"exact_instruction"`
	Context          ObjectiveContext `json:"objective_context"`
}

type ConversationObjectiveKindDecision struct {
	Schema string                    `json:"schema"`
	Kind   ConversationObjectiveKind `json:"kind"`
}

func NewConversationObjectiveKindJob(input ConversationObjectiveKindInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkConversationObjectiveKind, input, input.validate)
}

func (input ConversationObjectiveKindInput) validate() error {
	if strings.TrimSpace(input.ExactInstruction) == "" {
		return fmt.Errorf("conversation objective kind requires one non-blank exact instruction")
	}
	if len(input.ExactInstruction) > maxConversationInstructionBytes {
		return fmt.Errorf(
			"conversation exact instruction exceeds %d bytes", maxConversationInstructionBytes,
		)
	}
	if !utf8.ValidString(input.ExactInstruction) {
		return fmt.Errorf("conversation exact instruction is not valid UTF-8")
	}
	if strings.ContainsRune(input.ExactInstruction, '\x00') {
		return fmt.Errorf("conversation exact instruction contains NUL")
	}
	return input.Context.Validate()
}

func (decision ConversationObjectiveKindDecision) ValidateFor(input ConversationObjectiveKindInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ConversationObjectiveKindSchemaV1 {
		return fmt.Errorf(
			"conversation objective kind schema must be %q", ConversationObjectiveKindSchemaV1,
		)
	}
	switch decision.Kind {
	case ObjectiveKindAnswer,
		ObjectiveKindRepositoryRead,
		ObjectiveKindWorkspaceMutation,
		ObjectiveKindExternalAnswer,
		ObjectiveKindStory:
		return nil
	default:
		return fmt.Errorf("conversation objective kind %q is unsupported", decision.Kind)
	}
}

func DecodeConversationObjectiveKindDecision(
	input ConversationObjectiveKindInput,
	raw string,
) (ConversationObjectiveKindDecision, error) {
	if len(raw) > maxPortableCandidateBytes {
		return ConversationObjectiveKindDecision{}, fmt.Errorf(
			"conversation objective kind candidate exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	var decision ConversationObjectiveKindDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return ConversationObjectiveKindDecision{}, fmt.Errorf(
			"decode conversation objective kind decision: %w", err,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return ConversationObjectiveKindDecision{}, err
	}
	return decision, nil
}

func BuildConversationObjectiveKindPrompt(input ConversationObjectiveKindInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	context, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode objective context: %w", err)
	}
	return strings.Join([]string{
		"Classify one exact user instruction into exactly one registered code-owned objective kind.",
		"answer: answer without inspecting a repository or acquiring current external evidence.",
		"repository_read: inspect an existing repository without changing it.",
		"workspace_mutation: change a workspace and verify the change.",
		"external_answer: answer using current or externally acquired evidence.",
		"story: produce narrative or roleplay text.",
		"Classify only. Do not decompose, execute, or add responsibilities.",
		"OBJECTIVE_CONTEXT_JSON:\n" + string(context),
		"EXACT_INSTRUCTION:\n" + input.ExactInstruction,
	}, "\n\n"), nil
}

func ConversationObjectiveKindResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "kind"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ConversationObjectiveKindSchemaV1,
			},
			"kind": enumSchema(
				ObjectiveKindAnswer,
				ObjectiveKindRepositoryRead,
				ObjectiveKindWorkspaceMutation,
				ObjectiveKindExternalAnswer,
				ObjectiveKindStory,
			),
		},
	)
}
