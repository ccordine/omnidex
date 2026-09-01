package assemblyline

import (
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
	ObjectiveKindWorkspaceMutation ConversationObjectiveKind = "workspace_mutation"
	ObjectiveKindExternalAnswer    ConversationObjectiveKind = "external_answer"
	ObjectiveKindStory             ConversationObjectiveKind = "story"
	ObjectiveKindDatabaseRead      ConversationObjectiveKind = "database_read"
)

type ConversationObjectiveKindInput struct {
	ExactInstruction          string           `json:"exact_instruction"`
	Context                   ObjectiveContext `json:"objective_context"`
	DatabaseEvidenceAvailable bool             `json:"database_evidence_available"`
	KnownArtifactPaths        []string         `json:"known_artifact_paths"`
}

type ConversationObjectiveKindDecision struct {
	Schema string                    `json:"schema"`
	Kind   ConversationObjectiveKind `json:"kind"`
}

func NewConversationObjectiveKindJob(input ConversationObjectiveKindInput) (PortableJob, error) {
	return newPortableJob(WorkConversationObjectiveKind, input)
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
	if _, err := validateContextArtifactProvenance(
		"conversation objective kind", input.KnownArtifactPaths,
	); err != nil {
		return err
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
		ObjectiveKindWorkspaceMutation,
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
	if err := input.validate(); err != nil {
		return ConversationObjectiveKindDecision{}, err
	}
	choices, err := conversationObjectiveKindChoices(input)
	if err != nil {
		return ConversationObjectiveKindDecision{}, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return ConversationObjectiveKindDecision{}, err
	}
	decision := ConversationObjectiveKindDecision{
		Schema: ConversationObjectiveKindSchemaV1,
		Kind:   ConversationObjectiveKind(leaf),
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
	contextText, err := renderObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	provenance, err := validateContextArtifactProvenance(
		"conversation objective kind", input.KnownArtifactPaths,
	)
	if err != nil {
		return "", err
	}
	contextText, err = redactContextModelText(
		"conversation objective context", contextText, provenance,
	)
	if err != nil {
		return "", err
	}
	exactInstruction, err := redactContextModelText(
		"conversation exact instruction", input.ExactInstruction, provenance,
	)
	if err != nil {
		return "", err
	}
	choices, err := conversationObjectiveKindChoices(input)
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Which description exactly characterizes the objective of this one instruction?",
		[]string{
			"Choose the workspace-changing description only when satisfying the instruction requires that side effect.",
			"Objective context:\n" + contextText,
			"Exact instruction:\n" + exactInstruction,
		},
		choices,
	)
}

func conversationObjectiveKindChoices(
	input ConversationObjectiveKindInput,
) ([]OpaqueModelChoice, error) {
	descriptions := []struct {
		description string
		kind        ConversationObjectiveKind
	}{
		{
			description: "Converse directly, including greetings and small talk, or answer without acquiring current external evidence.",
			kind:        ObjectiveKindAnswer,
		},
		{
			description: "Satisfying the instruction requires changing a workspace and verifying the change.",
			kind:        ObjectiveKindWorkspaceMutation,
		},
		{
			description: "Produce narrative or roleplay text.",
			kind:        ObjectiveKindStory,
		},
	}
	if input.DatabaseEvidenceAvailable {
		descriptions = append(descriptions, struct {
			description string
			kind        ConversationObjectiveKind
		}{
			description: "Answer using the explicitly bound database because its records are required as evidence.",
			kind:        ObjectiveKindDatabaseRead,
		})
	}
	choices := make([]OpaqueModelChoice, 0, len(descriptions))
	for _, candidate := range descriptions {
		choice, err := NewOpaqueModelChoice(candidate.description, string(candidate.kind))
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}
