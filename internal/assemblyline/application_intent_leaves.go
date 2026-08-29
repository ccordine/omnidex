package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationProductContext          WorkKind = "application_product_context"
	WorkApplicationRequirementCoverage     WorkKind = "application_requirement_coverage"
	WorkApplicationRequirement             WorkKind = "application_requirement"
	MaxApplicationRequirementLeaves                 = maxRequirementCount
	ApplicationRequirementRemains                   = "REQUIREMENT_REMAINS"
	ApplicationNoUncoveredRequirement               = "NO_UNCOVERED_REQUIREMENT"
	ApplicationRequirementCoverageSchemaV1          = "omnidex.application-requirement-coverage.v1"
)

type ApplicationProductContextInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

type ApplicationRequirementCoverageInput struct {
	UserRequest          string             `json:"user_request"`
	Context              ApplicationContext `json:"context"`
	AcceptedRequirements []string           `json:"accepted_requirements"`
}

type ApplicationRequirementCoverageResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

type ApplicationRequirementCandidateInput struct {
	Authority ApplicationRequirementCoverageInput  `json:"authority"`
	Coverage  ApplicationRequirementCoverageResult `json:"coverage"`
}

type applicationRequirementLeafInputV1 struct {
	UserRequest          string             `json:"user_request"`
	Context              ApplicationContext `json:"context"`
	ProductContext       string             `json:"product_context"`
	AcceptedRequirements []string           `json:"accepted_requirements"`
}

// applicationRequirementLeafInputV2 is the exact flat payload frozen by
// renderer V7. Its product context remained code-owned payload authority even
// though the V7 prompt projected only request, facts, and retained leaves.
type applicationRequirementLeafInputV2 struct {
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
	input ApplicationRequirementCoverageInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCoverage, input, input.validate,
	)
}

func NewApplicationRequirementJob(
	input ApplicationRequirementCandidateInput,
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

func (input ApplicationRequirementCoverageInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	if input.AcceptedRequirements == nil {
		return fmt.Errorf("application requirement coverage requires a non-nil accepted set")
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

func (result ApplicationRequirementCoverageResult) ValidateFor(
	input ApplicationRequirementCoverageInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCoverageSchemaV1 {
		return fmt.Errorf(
			"application requirement coverage schema must be %q",
			ApplicationRequirementCoverageSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCoverageAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement coverage authority hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementRemains, ApplicationNoUncoveredRequirement:
		return nil
	default:
		return fmt.Errorf(
			"application requirement coverage value %q is not registered",
			result.Relation,
		)
	}
}

func (input ApplicationRequirementCandidateInput) validate() error {
	if err := input.Coverage.ValidateFor(input.Authority); err != nil {
		return fmt.Errorf("validate application requirement candidate coverage: %w", err)
	}
	if input.Coverage.Relation != ApplicationRequirementRemains {
		return fmt.Errorf(
			"application requirement generation requires code-established relation %q",
			ApplicationRequirementRemains,
		)
	}
	return nil
}

func (input applicationRequirementLeafInputV1) validate() error {
	if err := (ApplicationRequirementCoverageInput{
		UserRequest: input.UserRequest, Context: input.Context,
		AcceptedRequirements: input.AcceptedRequirements,
	}).validate(); err != nil {
		return err
	}
	return validateApplicationIntentText(
		"product context", input.ProductContext, maxApplicationProductBytes,
	)
}

func (input applicationRequirementLeafInputV2) validate() error {
	if err := (ApplicationRequirementCoverageInput{
		UserRequest: input.UserRequest, Context: input.Context,
		AcceptedRequirements: input.AcceptedRequirements,
	}).validate(); err != nil {
		return err
	}
	return validateApplicationIntentText(
		"product context", input.ProductContext, maxApplicationProductBytes,
	)
}

func applicationRequirementCoverageAuthoritySHA256(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement coverage authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}

func BuildApplicationProductContextPrompt(
	input ApplicationProductContextInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := renderApplicationContextModelProjection(input.UserRequest, input.Context)
	return strings.Join([]string{
		"Answer one semantic question: what concise product or domain identity is explicitly established by this software request and its established facts?",
		"Product context contains only the product identity, subject or domain, intended audience, and stated setting or purpose. Exclude requested qualities, capabilities, behaviors, user-visible elements, state or persistence, artifact or format constraints, accessibility or responsiveness, tests, build or deployment constraints, and implementation detail.",
		"Return only one concise product or domain identity phrase as raw prose. Do not prefix it with meta-language such as a description of product context. Do not return requirements, JSON, quotes, a label, Markdown, or commentary.",
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
	input ApplicationRequirementCoverageInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := applicationRequirementLeafProjection(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic relation: does the immutable request establish any explicit task-local runtime implementation requirement that is not semantically covered by the accepted requirement statements?",
		"A requirement for this fixed point is exactly one independently testable runtime outcome: one capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format that requires application source.",
		"Silently examine the immutable request in source order and compare every distinct runtime outcome separately. Items joined by commas, conjunctions, or a list remain separate outcomes. An accepted statement covers an outcome only when that exact outcome is semantically entailed; mentioning the same product, surface, or a neighboring outcome provides no coverage.",
		"For this question, do not count product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, generic test obligations, build or verification obligations, or deployment and continued-availability obligations.",
		"An observable output such as exporting CSV is a runtime behavior and is included. An implementation format such as using Rust, React, Jest, or a single-file project is excluded.",
		"Return REQUIREMENT_REMAINS when at least one included runtime outcome remains uncovered. Return NO_UNCOVERED_REQUIREMENT only when every included runtime outcome is covered.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
		"FINAL QUESTION:\nDoes even one independently testable included runtime outcome remain uncovered? Return only REQUIREMENT_REMAINS or NO_UNCOVERED_REQUIREMENT.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCoverageLeaf(
	input ApplicationRequirementCoverageInput,
	raw string,
) (ApplicationRequirementCoverageResult, error) {
	var zero ApplicationRequirementCoverageResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement coverage", raw, 32, false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCoverageAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCoverageResult{
		Schema:          ApplicationRequirementCoverageSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

// DecodeApplicationRequirementCoverageLeafForPortableRenderer validates one
// replay response against the exact payload schema owned by its renderer.
func DecodeApplicationRequirementCoverageLeafForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (string, error) {
	switch renderer {
	case PortableRendererV8:
		var input ApplicationRequirementCoverageInput
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
		result, err := DecodeApplicationRequirementCoverageLeaf(input, raw)
		if err != nil {
			return "", err
		}
		return result.Relation, nil
	case HistoricalPortableRendererV7:
		var input applicationRequirementLeafInputV2
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
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
	case HistoricalPortableRendererV6, HistoricalPortableRendererV5:
		var input applicationRequirementLeafInputV1
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
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
	default:
		return "", fmt.Errorf("portable renderer %q is not registered", renderer)
	}
}

func BuildApplicationRequirementPrompt(
	input ApplicationRequirementCandidateInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := applicationRequirementLeafProjection(input.Authority)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return the earliest uncovered explicit task-local runtime implementation requirement from the immutable request.",
		"The answer must describe exactly one independently testable runtime outcome: one capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format that requires application source.",
		"If the request joins outcomes with commas, conjunctions, or a list, return only the first uncovered outcome. Never return an umbrella construction statement, a list, or multiple actions, elements, qualities, or outcomes. A requirement that needs more than one independent runtime assertion is too broad.",
		"Do not return product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, generic test obligations, build or verification obligations, or deployment and continued-availability obligations.",
		"An observable output such as exporting CSV is a runtime behavior and may be returned. An implementation format such as using Rust, React, Jest, or a single-file project must not be returned.",
		"Faithfully paraphrase only that one outcome and use only the minimum product identity needed to understand it. Do not add implementation detail, unstated obligations, a broader product summary, or another requirement.",
		"Return only the requirement as raw prose. Do not return another requirement, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
		"CODE-ESTABLISHED UNCOVERED RELATION:\n" + input.Coverage.Relation,
		"FINAL QUESTION:\nWhat is the one earliest uncovered independently testable runtime outcome? Return exactly that one outcome, never a list or umbrella.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementLeaf(
	input ApplicationRequirementCandidateInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeApplicationRequirementText(input.Authority.AcceptedRequirements, raw)
}

// DecodeApplicationRequirementLeafForPortableRenderer validates one replay
// response against the exact generation payload schema owned by its renderer.
func DecodeApplicationRequirementLeafForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (string, error) {
	switch renderer {
	case PortableRendererV8:
		var input ApplicationRequirementCandidateInput
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
		return DecodeApplicationRequirementLeaf(input, raw)
	case HistoricalPortableRendererV7:
		var input applicationRequirementLeafInputV2
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
		if err := input.validate(); err != nil {
			return "", err
		}
		return decodeApplicationRequirementText(input.AcceptedRequirements, raw)
	case HistoricalPortableRendererV6, HistoricalPortableRendererV5:
		var input applicationRequirementLeafInputV1
		if err := decodePortablePayload(payload, &input); err != nil {
			return "", err
		}
		if err := input.validate(); err != nil {
			return "", err
		}
		return decodeApplicationRequirementText(input.AcceptedRequirements, raw)
	default:
		return "", fmt.Errorf("portable renderer %q is not registered", renderer)
	}
}

func decodeApplicationRequirementText(
	acceptedRequirements []string,
	raw string,
) (string, error) {
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
	for _, accepted := range acceptedRequirements {
		if leaf == accepted {
			return "", fmt.Errorf("application requirement duplicates an accepted statement")
		}
	}
	return leaf, nil
}

func applicationRequirementLeafProjection(
	input ApplicationRequirementCoverageInput,
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
