package assemblyline

import (
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
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	return strings.Join([]string{
		"Answer one semantic question: what concise product context is explicitly established by this software request and its established facts?",
		"Return only that product context as raw prose. Do not return requirements, implementation detail, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION PRODUCT INPUT:\n" + projection,
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
	projection, err := applicationRequirementLeafProjection(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the immutable request establish any explicit software requirement that is not semantically covered by the accepted requirement statements?",
		"Return REQUIREMENT_REMAINS when at least one explicit capability, behavior, user-visible element, artifact constraint, or technical-format constraint remains uncovered. Return NO_UNCOVERED_REQUIREMENT when every explicit requirement is covered.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
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
	projection, err := applicationRequirementLeafProjection(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one explicit software requirement from the immutable request that is not semantically covered by the accepted requirement statements.",
		"Choose the earliest uncovered requirement in source order. Faithfully paraphrase only that one requirement; do not add implementation detail or unstated obligations.",
		"Return only the requirement as raw prose. Do not return another requirement, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
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

func applicationRequirementLeafProjection(
	input ApplicationRequirementLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var projection strings.Builder
	projection.WriteString(renderApplicationContextModelProjection(
		input.UserRequest,
		input.Context,
	))
	fmt.Fprintf(&projection, "\nPRODUCT CONTEXT:\n%s\n", input.ProductContext)
	if len(input.AcceptedRequirements) == 0 {
		projection.WriteString("ACCEPTED REQUIREMENTS:\n(none)\n")
	} else {
		for index, requirement := range input.AcceptedRequirements {
			fmt.Fprintf(
				&projection,
				"ACCEPTED REQUIREMENT %d:\n%s\n",
				index+1,
				requirement,
			)
		}
	}
	if projection.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application requirement projection exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n"), nil
}
