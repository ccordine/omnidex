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
		"Answer one semantic relation: is any explicit task-local runtime outcome in the immutable request not covered by the accepted requirement statements?",
		"Compare every explicit runtime outcome separately in source order. Split comma, conjunction, and list items. An accepted statement covers an outcome only when it semantically entails the same behavior and affected object, response, state, quality, or output. If even one explicit runtime outcome has no matching accepted statement, return REQUIREMENT_REMAINS.",
		"A runtime outcome is one independently testable capability, observable behavior or quality, user-visible element, state or persistence behavior, response, or runtime data or output format requiring application source. A qualitative description or missing determining rule does not erase an explicitly requested outcome.",
		"A behavior literally named by a product name is explicit; behavior merely customary for a product category is not. Never infer an unasserted prerequisite, enabling behavior, ordinary effect, or likely consequence. Such unasserted behavior cannot create an uncovered outcome.",
		"Exclude product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, implementation format, generic test obligations, build or verification obligations, and deployment and continued-availability obligations. Apply exclusions clause by clause; an excluded clause never removes a neighboring runtime outcome. An observable output such as exporting CSV is a runtime behavior and is included. An implementation format such as using Rust, React, Jest, or a single-file project is excluded.",
		"A recorded zero delta is already covered by its indexed retained value and added no accepted outcome. It is evidence only, never a new requirement.",
		"Return NO_UNCOVERED_REQUIREMENT only when every explicit included runtime outcome has a matching accepted statement. Otherwise return REQUIREMENT_REMAINS.",
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
		"When the outcome produces a derived result, name the one semantic relation that determines it and the observable operands, conditions, and result meaning needed to compute an independent oracle. A quality claim about an output is not its determining rule. State an otherwise unstated relation only when the immutable request semantically entails exactly one determining rule and its operands, conditions, and result meaning for that outcome; ambiguity means the relation is missing.",
		"If the request joins outcomes with commas, conjunctions, or a list, return only the first uncovered outcome. Never return an umbrella construction statement, a list, or multiple actions, elements, qualities, or outcomes. A requirement that needs more than one independent runtime assertion is too broad.",
		"A product name asserts only an action or result literally denoted by its words; never infer behavior merely customary for that product category. Return a named capability or element as one outcome. Do not emit an unasserted prerequisite, enabling behavior, ordinary effect, or likely consequence; when it is inherent in an accepted capability's ordinary meaning, it is already covered.",
		"Do not return product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, generic test obligations, build or verification obligations, or deployment and continued-availability obligations.",
		"Do not repeat or paraphrase an accepted requirement, a recorded zero-delta candidate, or a listed excluded non-runtime candidate.",
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
