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
	ObjectiveKindDatabaseRead      ConversationObjectiveKind = "database_read"
)

type ConversationObjectiveKindInput struct {
	ExactInstruction          string           `json:"exact_instruction"`
	Context                   ObjectiveContext `json:"objective_context"`
	DatabaseEvidenceAvailable bool             `json:"database_evidence_available"`
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
	case ObjectiveKindDatabaseRead:
		if !input.DatabaseEvidenceAvailable {
			return fmt.Errorf("database-read objective requires an explicit data-source binding")
		}
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
	modelContext, err := projectObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	context, err := json.Marshal(modelContext)
	if err != nil {
		return "", fmt.Errorf("encode objective context: %w", err)
	}
	lines := []string{
		"Classify one exact user instruction into exactly one registered code-owned objective kind.",
		"answer: converse directly, including greetings and small talk, or answer without inspecting a repository or acquiring current external evidence.",
		"repository_read: satisfying the instruction requires inspecting an existing repository without changing it.",
		"workspace_mutation: satisfying the instruction requires changing a workspace and verifying the change.",
		"external_answer: satisfying the instruction requires current or externally acquired evidence, including an explicit web-search or research request.",
		"story: produce narrative or roleplay text.",
		"Choose repository_read, workspace_mutation, or external_answer only when the instruction requires that corresponding evidence or side effect; otherwise choose answer or story.",
	}
	if input.DatabaseEvidenceAvailable {
		lines = append(lines, "database_read: answer using the explicitly bound database when its records are required as evidence.")
	}
	lines = append(lines,
		"Return the one registered semantic objective kind that exactly describes this instruction.",
		"OBJECTIVE_CONTEXT_JSON:\n"+string(context),
		"EXACT_INSTRUCTION:\n"+input.ExactInstruction,
	)
	return strings.Join(lines, "\n\n"), nil
}

func ConversationObjectiveKindResponseSchema(input ConversationObjectiveKindInput) map[string]any {
	kinds := []ConversationObjectiveKind{
		ObjectiveKindAnswer,
		ObjectiveKindRepositoryRead,
		ObjectiveKindWorkspaceMutation,
		ObjectiveKindExternalAnswer,
		ObjectiveKindStory,
	}
	if input.DatabaseEvidenceAvailable {
		kinds = append(kinds, ObjectiveKindDatabaseRead)
	}
	return objectSchema(
		[]string{"schema", "kind"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ConversationObjectiveKindSchemaV1,
			},
			"kind": enumSchema(kinds...),
		},
	)
}
