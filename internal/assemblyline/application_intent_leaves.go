package assemblyline

import "strings"

const WorkApplicationProductContext WorkKind = "application_product_context"

type ApplicationProductContextInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

func NewApplicationProductContextJob(
	input ApplicationProductContextInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationProductContext, input, input.validate,
	)
}

func (input ApplicationProductContextInput) validate() error {
	return (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate()
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
	projection, err := applicationRequirementCoverageProjection(input)
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

func BuildApplicationRequirementPrompt(
	input ApplicationRequirementCandidateInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := applicationRequirementGenerationProjection(input.Authority)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return the earliest uncovered explicit task-local runtime implementation requirement from the immutable request.",
		"The answer must describe exactly one independently testable runtime outcome: one capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format that requires application source.",
		"If the request joins outcomes with commas, conjunctions, or a list, return only the first uncovered outcome. Never return an umbrella construction statement, a list, or multiple actions, elements, qualities, or outcomes. A requirement that needs more than one independent runtime assertion is too broad.",
		"Do not return product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, generic test obligations, build or verification obligations, or deployment and continued-availability obligations.",
		"Do not repeat or paraphrase any listed excluded non-runtime candidate.",
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
	return decodeCurrentApplicationRequirementText(raw)
}

func decodeCurrentApplicationRequirementText(raw string) (string, error) {
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
	return leaf, nil
}
