package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationContextNeedCoverage WorkKind = "application_context_need_coverage"
	WorkApplicationContextNeedQuestion WorkKind = "application_context_need_question"
	ApplicationContextNeedSchemaV1              = "omnidex.application-context-needs.v1"

	ApplicationContextNeedRemains     = "CONTEXT_NEED_REMAINS"
	ApplicationNoUncoveredContextNeed = "NO_UNCOVERED_CONTEXT_NEED"
)

// ApplicationContextNeedLeafInput is the immutable authority for one
// coverage relation or one missing-fact question. AcceptedQuestions is
// assembled and owned by code; it is not a model-authored plan.
type ApplicationContextNeedLeafInput struct {
	UserRequest       string             `json:"user_request"`
	Context           ApplicationContext `json:"context"`
	AcceptedQuestions []string           `json:"accepted_questions"`
}

func NewApplicationContextNeedCoverageJob(
	input ApplicationContextNeedLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationContextNeedCoverage, input, input.validate,
	)
}

func NewApplicationContextNeedQuestionJob(
	input ApplicationContextNeedLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationContextNeedQuestion, input, input.validate,
	)
}

func (input ApplicationContextNeedLeafInput) validate() error {
	base := ApplicationContextNeedInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}
	if err := base.validate(); err != nil {
		return err
	}
	if input.AcceptedQuestions == nil {
		return fmt.Errorf("application context need leaf requires a non-nil accepted set")
	}
	return (ApplicationContextNeedDecision{
		Schema:    ApplicationContextNeedSchemaV1,
		Questions: append([]string{}, input.AcceptedQuestions...),
	}).Validate()
}

func BuildApplicationContextNeedCoveragePrompt(
	input ApplicationContextNeedLeafInput,
) (string, error) {
	projection, err := renderApplicationContextNeedLeafProjection(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: is there one necessary missing-fact question that is not semantically covered by the accepted questions?",
		"A necessary question asks for one repository fact required to interpret the immutable request and not already answered by the established facts or accepted questions.",
		"Output grammar: CONTEXT_NEED_REMAINS | NO_UNCOVERED_CONTEXT_NEED",
		"APPLICATION CONTEXT NEED INPUT:\n" + projection,
	}, "\n\n"), nil
}

func DecodeApplicationContextNeedCoverageLeaf(
	input ApplicationContextNeedLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application context need coverage", raw, 32, false,
	)
	if err != nil {
		return "", err
	}
	switch leaf {
	case ApplicationContextNeedRemains, ApplicationNoUncoveredContextNeed:
		return leaf, nil
	default:
		return "", fmt.Errorf(
			"application context need coverage value %q is not registered", leaf,
		)
	}
}

func BuildApplicationContextNeedQuestionPrompt(
	input ApplicationContextNeedLeafInput,
) (string, error) {
	projection, err := renderApplicationContextNeedLeafProjection(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one necessary missing-fact question that is not semantically covered by the established facts or accepted questions.",
		"The result is one raw interrogative sentence asking for one repository fact required to interpret the immutable software request faithfully.",
		"APPLICATION CONTEXT NEED INPUT:\n" + projection,
	}, "\n\n"), nil
}

func DecodeApplicationContextNeedQuestionLeaf(
	input ApplicationContextNeedLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application context need question", raw,
		maxApplicationEvidenceQuestionBytes, false,
	)
	if err != nil {
		return "", err
	}
	questions := append(append([]string{}, input.AcceptedQuestions...), leaf)
	if err := (ApplicationContextNeedDecision{
		Schema: ApplicationContextNeedSchemaV1, Questions: questions,
	}).Validate(); err != nil {
		return "", err
	}
	return leaf, nil
}

func AssembleApplicationContextNeedDecision(
	input ApplicationContextNeedInput,
	questions []string,
) (ApplicationContextNeedDecision, error) {
	if err := input.validate(); err != nil {
		return ApplicationContextNeedDecision{}, err
	}
	decision := ApplicationContextNeedDecision{
		Schema:    ApplicationContextNeedSchemaV1,
		Questions: append([]string{}, questions...),
	}
	if err := decision.Validate(); err != nil {
		return ApplicationContextNeedDecision{}, err
	}
	return decision, nil
}

func renderApplicationContextNeedLeafProjection(
	input ApplicationContextNeedLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var projection strings.Builder
	projection.WriteString(renderApplicationContextModelProjection(
		input.UserRequest,
		input.Context,
	))
	projection.WriteByte('\n')
	if len(input.AcceptedQuestions) == 0 {
		projection.WriteString("ACCEPTED QUESTIONS:\n(none)\n")
	} else {
		for index, question := range input.AcceptedQuestions {
			fmt.Fprintf(&projection, "ACCEPTED QUESTION %d:\n%s\n", index+1, question)
		}
	}
	if projection.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application context need projection exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n"), nil
}
