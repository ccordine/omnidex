package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationContextQuestionRelation WorkKind = "application_context_question_relation"

	ApplicationContextQuestionsSameFact     = "SAME_REPOSITORY_FACT_QUESTION"
	ApplicationContextQuestionsDistinctFact = "DISTINCT_REPOSITORY_FACT_QUESTIONS"

	ApplicationContextQuestionRelationSchemaV1 = "omnidex.application-context-question-relation.v1"
)

// ApplicationContextQuestionRelationInput contains exactly one queued
// candidate and one already accepted question. The accepted question remains
// immutable authority; the relation may only cause code to discard the queued
// candidate or compare it with the next accepted question.
type ApplicationContextQuestionRelationInput struct {
	CandidateQuestion string `json:"candidate_question"`
	AcceptedQuestion  string `json:"accepted_question"`
}

type ApplicationContextQuestionRelationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationContextQuestionRelationJob(
	input ApplicationContextQuestionRelationInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkApplicationContextQuestionRelation, input,
	)
}

func (input ApplicationContextQuestionRelationInput) validate() error {
	if err := validateApplicationContextQuestion(0, input.CandidateQuestion); err != nil {
		return err
	}
	if err := validateApplicationContextQuestion(1, input.AcceptedQuestion); err != nil {
		return err
	}
	if input.CandidateQuestion == input.AcceptedQuestion {
		return fmt.Errorf("exact application context question duplicates must be skipped by code")
	}
	return nil
}

func BuildApplicationContextQuestionRelationPrompt(
	input ApplicationContextQuestionRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one pairwise semantic identity relation: do the exact candidate question and the exact already accepted question ask for the same repository fact?",
		"Return SAME_REPOSITORY_FACT_QUESTION when truthful answers to the two questions would establish the same repository fact despite different wording. Return DISTINCT_REPOSITORY_FACT_QUESTIONS when each question requires a separately establishable fact.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"EXACT CANDIDATE REPOSITORY-FACT QUESTION:\n" + input.CandidateQuestion,
		"EXACT ALREADY ACCEPTED REPOSITORY-FACT QUESTION:\n" + input.AcceptedQuestion,
	}, "\n\n"), nil
}

func DecodeApplicationContextQuestionRelationResult(
	input ApplicationContextQuestionRelationInput,
	raw string,
) (ApplicationContextQuestionRelationResult, error) {
	var zero ApplicationContextQuestionRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application context question relation",
		raw,
		maximumStringBytes(
			ApplicationContextQuestionsSameFact,
			ApplicationContextQuestionsDistinctFact,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationContextQuestionRelationAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationContextQuestionRelationResult{
		Schema:          ApplicationContextQuestionRelationSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result ApplicationContextQuestionRelationResult) ValidateFor(
	input ApplicationContextQuestionRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationContextQuestionRelationSchemaV1 {
		return fmt.Errorf(
			"application context question relation schema must be %q",
			ApplicationContextQuestionRelationSchemaV1,
		)
	}
	authoritySHA256, err := applicationContextQuestionRelationAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application context question relation authority hash does not match")
	}
	switch result.Relation {
	case ApplicationContextQuestionsSameFact, ApplicationContextQuestionsDistinctFact:
		return nil
	default:
		return fmt.Errorf(
			"application context question relation %q is not registered",
			result.Relation,
		)
	}
}

func applicationContextQuestionRelationAuthoritySHA256(
	input ApplicationContextQuestionRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application context question relation authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
