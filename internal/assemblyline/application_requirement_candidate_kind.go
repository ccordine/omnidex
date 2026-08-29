package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateKind WorkKind = "application_requirement_candidate_kind"

	ApplicationRequirementCandidateTaskLocal  = "TASK_LOCAL_RUNTIME_OUTCOME"
	ApplicationRequirementCandidateNonRuntime = "NON_RUNTIME_CONSTRAINT"

	ApplicationRequirementCandidateKindSchemaV1 = "omnidex.application-requirement-candidate-kind.v1"
)

type ApplicationRequirementCandidateKindInput struct {
	Candidate string `json:"candidate"`
}

type ApplicationRequirementCandidateKindResult struct {
	Schema          string `json:"schema"`
	CandidateSHA256 string `json:"candidate_sha256"`
	Relation        string `json:"relation"`
}

func NewApplicationRequirementCandidateKindJob(
	input ApplicationRequirementCandidateKindInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateKind, input, input.validate,
	)
}

func (input ApplicationRequirementCandidateKindInput) validate() error {
	return validateApplicationIntentText(
		"application requirement candidate", input.Candidate, maxRequirementQuoteBytes,
	)
}

func (result ApplicationRequirementCandidateKindResult) ValidateFor(
	input ApplicationRequirementCandidateKindInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCandidateKindSchemaV1 {
		return fmt.Errorf(
			"application requirement candidate kind schema must be %q",
			ApplicationRequirementCandidateKindSchemaV1,
		)
	}
	if result.CandidateSHA256 != ExactObjectiveContextSHA(input.Candidate) {
		return fmt.Errorf("application requirement candidate kind hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementCandidateTaskLocal,
		ApplicationRequirementCandidateNonRuntime:
		return nil
	default:
		return fmt.Errorf(
			"application requirement candidate kind value %q is not registered",
			result.Relation,
		)
	}
}

func BuildApplicationRequirementCandidateKindPrompt(
	input ApplicationRequirementCandidateKindInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic classification about the exact candidate below: does it contain only task-local runtime-outcome content that requires application source, or does it express a non-runtime constraint?",
		"A task-local runtime outcome is a capability, observable behavior, user-visible element, observable quality, state or persistence behavior, or runtime data or output format. An observable output such as exporting CSV is a runtime outcome.",
		"A non-runtime constraint is product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact shape, a generic test obligation, a build or verification obligation, or a deployment and continued-availability obligation.",
		"This classification does not decide whether the candidate contains one runtime outcome or multiple runtime outcomes. A candidate containing multiple runtime outcomes but no non-runtime constraint is TASK_LOCAL_RUNTIME_OUTCOME; cardinality is a separate question.",
		"Classify only the exact candidate. Do not rewrite it, infer another requirement, or use the surrounding request.",
		"Return TASK_LOCAL_RUNTIME_OUTCOME when the candidate contains only task-local runtime-outcome content. Return NON_RUNTIME_CONSTRAINT when it expresses a non-runtime constraint.",
		"Return exactly that raw registered value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"EXACT REQUIREMENT CANDIDATE:\n" + input.Candidate,
		"FINAL QUESTION:\nDoes this exact candidate contain only task-local runtime-outcome content, or does it express a non-runtime constraint? Return only TASK_LOCAL_RUNTIME_OUTCOME or NON_RUNTIME_CONSTRAINT.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateKindResult(
	input ApplicationRequirementCandidateKindInput,
	raw string,
) (ApplicationRequirementCandidateKindResult, error) {
	var zero ApplicationRequirementCandidateKindResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate kind", raw, 32, false,
	)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateKindResult{
		Schema:          ApplicationRequirementCandidateKindSchemaV1,
		CandidateSHA256: ExactObjectiveContextSHA(input.Candidate),
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

// DecodeApplicationRequirementCandidateKindResultForPortableRenderer validates
// one replay response against the sole renderer that owns this work kind.
func DecodeApplicationRequirementCandidateKindResultForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (ApplicationRequirementCandidateKindResult, error) {
	var zero ApplicationRequirementCandidateKindResult
	if renderer != PortableRendererV1 {
		return zero, fmt.Errorf(
			"portable work kind %q requires renderer %q",
			WorkApplicationRequirementCandidateKind, PortableRendererV1,
		)
	}
	var input ApplicationRequirementCandidateKindInput
	if err := decodePortablePayload(payload, &input); err != nil {
		return zero, err
	}
	return DecodeApplicationRequirementCandidateKindResult(input, raw)
}
