package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationContextQuestionNecessity WorkKind = "application_context_question_necessity"

	ApplicationContextQuestionNecessary    = "NECESSARY_UNRESOLVED_REPOSITORY_FACT"
	ApplicationContextQuestionNotNecessary = "NOT_NECESSARY_OR_ALREADY_RESOLVED"

	ApplicationContextQuestionNecessitySchemaV2 = "omnidex.application-context-question-necessity.v2"
)

type ApplicationContextQuestionNecessityInput struct {
	Authority      ApplicationContextQuestionInventoryInput `json:"authority"`
	Inventory      ApplicationContextQuestionInventory      `json:"inventory"`
	CandidateIndex int                                      `json:"candidate_index"`
	CurrentContext ApplicationContext                       `json:"current_context"`
}

type ApplicationContextQuestionNecessityResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationContextQuestionNecessityJob(
	input ApplicationContextQuestionNecessityInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationContextQuestionNecessity,
		input,
	)
}

func (input ApplicationContextQuestionNecessityInput) validate() error {
	if err := input.Inventory.ValidateFor(input.Authority); err != nil {
		return err
	}
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return fmt.Errorf("application context question candidate index is outside inventory")
	}
	if err := input.CurrentContext.Validate(); err != nil {
		return err
	}
	if err := validateApplicationContextExtension(
		input.Authority.Context,
		input.CurrentContext,
	); err != nil {
		return err
	}
	return nil
}

func (input ApplicationContextQuestionNecessityInput) candidate() string {
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return ""
	}
	return input.Inventory.Candidates[input.CandidateIndex]
}

func BuildApplicationContextQuestionNecessityPrompt(
	input ApplicationContextQuestionNecessityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic authorization relation: under the immutable software request and current established facts, does the exact candidate ask for one still-unresolved repository fact that is necessary to interpret the request faithfully?",
		"Return NECESSARY_UNRESOLVED_REPOSITORY_FACT only when both necessity and unresolvedness hold. Return NOT_NECESSARY_OR_ALREADY_RESOLVED when the fact is already established or the candidate is optional, speculative, customary, implementation-specific, or unrelated to faithful request interpretation.",
		"Evaluate only necessity and unresolvedness for this exact candidate.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"IMMUTABLE REQUEST AND CURRENT ESTABLISHED FACTS:\n" + renderApplicationContextModelProjection(
			input.Authority.UserRequest,
			input.CurrentContext,
		),
		"EXACT CANDIDATE REPOSITORY-FACT QUESTION:\n" + input.candidate(),
		"FINAL QUESTION:\nIs this exact candidate a necessary, still-unresolved repository-fact question? Return only NECESSARY_UNRESOLVED_REPOSITORY_FACT or NOT_NECESSARY_OR_ALREADY_RESOLVED.",
	}, "\n\n"), nil
}

func DecodeApplicationContextQuestionNecessityResult(
	input ApplicationContextQuestionNecessityInput,
	raw string,
) (ApplicationContextQuestionNecessityResult, error) {
	var zero ApplicationContextQuestionNecessityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application context question necessity",
		raw,
		maximumStringBytes(
			ApplicationContextQuestionNecessary,
			ApplicationContextQuestionNotNecessary,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationContextQuestionNecessityAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationContextQuestionNecessityResult{
		Schema:          ApplicationContextQuestionNecessitySchemaV2,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result ApplicationContextQuestionNecessityResult) ValidateFor(
	input ApplicationContextQuestionNecessityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationContextQuestionNecessitySchemaV2 {
		return fmt.Errorf(
			"application context question necessity schema must be %q",
			ApplicationContextQuestionNecessitySchemaV2,
		)
	}
	authoritySHA256, err := applicationContextQuestionNecessityAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application context question necessity authority hash does not match")
	}
	switch result.Relation {
	case ApplicationContextQuestionNecessary, ApplicationContextQuestionNotNecessary:
		return nil
	default:
		return fmt.Errorf(
			"application context question necessity relation %q is not registered",
			result.Relation,
		)
	}
}

func validateApplicationContextExtension(
	initial ApplicationContext,
	current ApplicationContext,
) error {
	if err := initial.Validate(); err != nil {
		return err
	}
	if current.Schema != initial.Schema ||
		current.RequestSHA256 != initial.RequestSHA256 ||
		len(current.Facts) < len(initial.Facts) {
		return fmt.Errorf("current application context is not an extension of inventory authority")
	}
	for index := range initial.Facts {
		if current.Facts[index] != initial.Facts[index] {
			return fmt.Errorf("current application context changed initial fact %d", index)
		}
	}
	return nil
}

func applicationContextQuestionNecessityAuthoritySHA256(
	input ApplicationContextQuestionNecessityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application context question necessity authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
