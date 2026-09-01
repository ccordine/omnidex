package assemblyline

import (
	"fmt"
	"strings"
)

const ContextMinificationSchemaV1 = "omnidex.context-minification.v1"

type ContextMinificationInput struct {
	ExactInstruction    string                      `json:"exact_instruction"`
	SelectedAuthorities []ContextCandidateAuthority `json:"selected_authorities"`
	Scope               ContextScope                `json:"scope,omitempty"`
	KnownArtifactPaths  []string                    `json:"known_artifact_paths"`
}

type ContextMinificationDecision struct {
	Schema         string `json:"schema"`
	MinimalContext string `json:"minimal_context"`
}

func NewContextMinificationJob(input ContextMinificationInput) (PortableJob, error) {
	return newPortableJob(WorkContextMinification, input)
}

func (input ContextMinificationInput) validate() error {
	if err := input.Scope.Validate(); err != nil {
		return err
	}
	if err := validateContextExactInstruction(input.ExactInstruction); err != nil {
		return err
	}
	if _, err := validateContextArtifactProvenance(
		"context minification", input.KnownArtifactPaths,
	); err != nil {
		return err
	}
	return validateContextCandidateAuthorities(
		"context minification",
		input.SelectedAuthorities,
		MaxContextMinificationAuthorities,
		MaxContextMinificationProjectionBytes,
	)
}

func (decision ContextMinificationDecision) ValidateFor(input ContextMinificationInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ContextMinificationSchemaV1 {
		return fmt.Errorf("context minification schema must be %q", ContextMinificationSchemaV1)
	}
	if err := validateContextText("minimal context", decision.MinimalContext, MaxContextMinifiedBytes); err != nil {
		return err
	}
	provenance, err := validateContextArtifactProvenance(
		"context minification", input.KnownArtifactPaths,
	)
	if err != nil {
		return err
	}
	return validateContextRawModelOutput(
		"context minification decision", decision.MinimalContext, provenance,
	)
}

func DecodeContextMinificationDecision(
	input ContextMinificationInput,
	raw string,
) (ContextMinificationDecision, error) {
	provenance, err := validateContextArtifactProvenance(
		"context minification", input.KnownArtifactPaths,
	)
	if err != nil {
		return ContextMinificationDecision{}, err
	}
	if err := validateContextRawModelOutput(
		"context minification raw result", raw, provenance,
	); err != nil {
		return ContextMinificationDecision{}, err
	}
	leaf, err := decodeOrdinarySemanticText("context minification", raw, MaxContextMinifiedBytes)
	if err != nil {
		return ContextMinificationDecision{}, err
	}
	decision := ContextMinificationDecision{
		Schema:         ContextMinificationSchemaV1,
		MinimalContext: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return ContextMinificationDecision{}, err
	}
	return decision, nil
}

func BuildContextMinificationPrompt(input ContextMinificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	provenance, err := validateContextArtifactProvenance(
		"context minification", input.KnownArtifactPaths,
	)
	if err != nil {
		return "", err
	}
	exactInstruction, err := redactContextModelText(
		"context minification exact instruction", input.ExactInstruction, provenance,
	)
	if err != nil {
		return "", err
	}
	selectedContext := make([]string, len(input.SelectedAuthorities))
	for index, authority := range input.SelectedAuthorities {
		content, err := redactContextModelText(
			"context minification selected content", authority.Content, provenance,
		)
		if err != nil {
			return "", fmt.Errorf("authority %s: %w", authority.CandidateID, err)
		}
		selectedContext[index] = content
	}
	sections := []string{
		"What is the smallest context needed to understand or answer this current instruction? Keep necessary referents, actors, actions, negations, and temporal relationships.",
		"Current instruction:\n" + exactInstruction,
	}
	for _, content := range selectedContext {
		sections = append(sections, "Relevant context:\n"+content)
	}
	return strings.Join(sections, "\n\n"), nil
}
