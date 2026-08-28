package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkApplicationProductContext      WorkKind = "application_product_context"
	WorkApplicationRequirementCoverage WorkKind = "application_requirement_coverage"
	WorkApplicationRequirement         WorkKind = "application_requirement"
	MaxApplicationRequirementLeaves             = maxRequirementCount
	ApplicationRequirementRemains               = "REQUIREMENT_REMAINS"
	ApplicationNoUncoveredRequirement           = "NO_UNCOVERED_REQUIREMENT"
)

type ApplicationProductContextInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

type ApplicationRequirementLeafInput struct {
	UserRequest          string             `json:"user_request"`
	Context              ApplicationContext `json:"context"`
	ProductContext       string             `json:"product_context"`
	AcceptedRequirements []string           `json:"accepted_requirements"`
}

func NewApplicationProductContextJob(
	input ApplicationProductContextInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationProductContext, input, input.validate,
	)
}

func NewApplicationRequirementCoverageJob(
	input ApplicationRequirementLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCoverage, input, input.validate,
	)
}

func NewApplicationRequirementJob(
	input ApplicationRequirementLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirement, input, input.validate,
	)
}

func (input ApplicationProductContextInput) validate() error {
	return (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate()
}

func (input ApplicationRequirementLeafInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"product context", input.ProductContext, maxApplicationProductBytes,
	); err != nil {
		return err
	}
	if input.AcceptedRequirements == nil {
		return fmt.Errorf("application requirement leaf requires a non-nil accepted set")
	}
	if len(input.AcceptedRequirements) > maxRequirementCount {
		return fmt.Errorf(
			"application requirement leaf exceeds %d accepted requirements",
			maxRequirementCount,
		)
	}
	seen := make(map[string]struct{}, len(input.AcceptedRequirements))
	for index, requirement := range input.AcceptedRequirements {
		if err := validateApplicationIntentText(
			"requirement statement", requirement, maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("accepted application requirement %d: %w", index, err)
		}
		if _, duplicate := seen[requirement]; duplicate {
			return fmt.Errorf("accepted application requirement %d is duplicated", index)
		}
		seen[requirement] = struct{}{}
	}
	return nil
}

func BuildApplicationProductContextPrompt(
	input ApplicationProductContextInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	context, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode application product context authority: %w", err)
	}
	return strings.Join([]string{
		"Answer one semantic question: what concise product context is explicitly established by this software request and its authoritative facts?",
		"Return only that product context as raw prose. Do not return requirements, implementation detail, JSON, quotes, a label, Markdown, or commentary.",
		"AUTHORITATIVE_CONTEXT:\n" + string(context),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func DecodeApplicationProductContextLeaf(
	input ApplicationProductContextInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application product context", raw, maxApplicationProductBytes, true,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationIntentText(
		"product context", leaf, maxApplicationProductBytes,
	); err != nil {
		return "", err
	}
	return leaf, nil
}

func BuildApplicationRequirementCoveragePrompt(
	input ApplicationRequirementLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationRequirementLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the immutable request establish any explicit software requirement that is not semantically covered by the accepted requirement statements?",
		"Return REQUIREMENT_REMAINS when at least one explicit capability, behavior, user-visible element, artifact constraint, or technical-format constraint remains uncovered. Return NO_UNCOVERED_REQUIREMENT when every explicit requirement is covered.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_REQUIREMENT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCoverageLeaf(
	input ApplicationRequirementLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement coverage", raw, 32, false,
	)
	if err != nil {
		return "", err
	}
	switch leaf {
	case ApplicationRequirementRemains, ApplicationNoUncoveredRequirement:
		return leaf, nil
	default:
		return "", fmt.Errorf(
			"application requirement coverage value %q is not registered", leaf,
		)
	}
}

func BuildApplicationRequirementPrompt(
	input ApplicationRequirementLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := applicationRequirementLeafAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one explicit software requirement from the immutable request that is not semantically covered by the accepted requirement statements.",
		"Choose the earliest uncovered requirement in source order. Faithfully paraphrase only that one requirement; do not add implementation detail or unstated obligations.",
		"Return only the requirement as raw prose. Do not return another requirement, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_REQUIREMENT_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeApplicationRequirementLeaf(
	input ApplicationRequirementLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement", raw, maxRequirementQuoteBytes, true,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationIntentText(
		"requirement statement", leaf, maxRequirementQuoteBytes,
	); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedRequirements {
		if leaf == accepted {
			return "", fmt.Errorf("application requirement duplicates an accepted statement")
		}
	}
	return leaf, nil
}

func applicationRequirementLeafAuthority(
	input ApplicationRequirementLeafInput,
) (string, error) {
	raw, err := json.Marshal(struct {
		UserRequest          string             `json:"user_request"`
		Context              ApplicationContext `json:"context"`
		ProductContext       string             `json:"product_context"`
		AcceptedRequirements []string           `json:"accepted_requirements"`
	}{
		UserRequest: input.UserRequest, Context: input.Context,
		ProductContext:       input.ProductContext,
		AcceptedRequirements: append([]string{}, input.AcceptedRequirements...),
	})
	if err != nil {
		return "", fmt.Errorf("encode application requirement authority: %w", err)
	}
	return string(raw), nil
}
