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
	authority, err := renderApplicationContextNeedLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: is there one necessary missing-fact question that is not semantically covered by the accepted questions?",
		"A necessary question asks for evidence required to interpret the immutable software request faithfully and not already answered by the authoritative facts. It never proposes an operation, command, artifact identity, implementation, architecture, plan, or completion claim.",
		"Return CONTEXT_NEED_REMAINS when at least one such question remains. Return NO_UNCOVERED_CONTEXT_NEED when none remains.",
		"Return exactly that registered raw value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_CONTEXT_NEED_AUTHORITY:\n" + authority,
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
	authority, err := renderApplicationContextNeedLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one necessary missing-fact question that is not semantically covered by the accepted questions.",
		"The question must request one evidence fact needed to interpret the immutable software request faithfully. It must not propose an operation, command, artifact identity, implementation, architecture, plan, or completion claim.",
		"Return only the question as one raw line. Do not return JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_CONTEXT_NEED_AUTHORITY:\n" + authority,
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

func renderApplicationContextNeedLeafAuthority(
	input ApplicationContextNeedLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var authority strings.Builder
	fmt.Fprintf(&authority, "IMMUTABLE USER REQUEST:\n%s\n", input.UserRequest)
	fmt.Fprintf(&authority, "WORKSPACE STATE:\n%s\n", input.Context.WorkspaceState)
	for index, fact := range input.Context.Facts {
		fmt.Fprintf(
			&authority, "AUTHORITATIVE FACT %d (%s; %s):\n%s\n",
			index+1, fact.Kind, fact.Authority, fact.Value,
		)
	}
	if len(input.AcceptedQuestions) == 0 {
		authority.WriteString("ACCEPTED QUESTIONS:\n(none)\n")
	} else {
		for index, question := range input.AcceptedQuestions {
			fmt.Fprintf(&authority, "ACCEPTED QUESTION %d:\n%s\n", index+1, question)
		}
	}
	if authority.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application context need authority exceeds %d bytes", maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(authority.String(), "\n"), nil
}
