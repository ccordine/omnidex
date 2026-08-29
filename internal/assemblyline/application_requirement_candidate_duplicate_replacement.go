package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateDuplicateReplacement WorkKind = "application_requirement_candidate_duplicate_replacement"

	ApplicationRequirementDuplicateAcceptedRequirement         = "ACCEPTED_REQUIREMENT"
	ApplicationRequirementDuplicateExcludedNonRuntimeCandidate = "EXCLUDED_NON_RUNTIME_CANDIDATE"
	ApplicationRequirementDuplicateCandidateDefect             = "EXACT_DUPLICATE_CANDIDATE"
)

type ApplicationRequirementCandidateDuplicateIdentity struct {
	Set   string `json:"set"`
	Index int    `json:"index"`
}

type ApplicationRequirementCandidateDuplicateReplacementInput struct {
	GenerationAuthority ApplicationRequirementCandidateInput             `json:"generation_authority"`
	CurrentCandidate    string                                           `json:"current_candidate"`
	Duplicate           ApplicationRequirementCandidateDuplicateIdentity `json:"duplicate"`
	Defect              string                                           `json:"defect"`
}

func NewApplicationRequirementCandidateDuplicateReplacementJob(
	input ApplicationRequirementCandidateDuplicateReplacementInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateDuplicateReplacement, input, input.validate,
	)
}

func (input ApplicationRequirementCandidateDuplicateReplacementInput) validate() error {
	if err := input.GenerationAuthority.validate(); err != nil {
		return fmt.Errorf(
			"validate application requirement duplicate replacement generation authority: %w",
			err,
		)
	}
	if err := validateApplicationIntentText(
		"application requirement duplicate candidate", input.CurrentCandidate,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	var candidates []string
	switch input.Duplicate.Set {
	case ApplicationRequirementDuplicateAcceptedRequirement:
		candidates = input.GenerationAuthority.Authority.AcceptedRequirements
	case ApplicationRequirementDuplicateExcludedNonRuntimeCandidate:
		candidates = input.GenerationAuthority.Authority.ExcludedCandidates
	default:
		return fmt.Errorf(
			"application requirement duplicate set %q is not registered",
			input.Duplicate.Set,
		)
	}
	if input.Duplicate.Index < 0 || input.Duplicate.Index >= len(candidates) {
		return fmt.Errorf(
			"application requirement duplicate index %d is outside %s",
			input.Duplicate.Index,
			input.Duplicate.Set,
		)
	}
	if candidates[input.Duplicate.Index] != input.CurrentCandidate {
		return fmt.Errorf(
			"application requirement duplicate identity does not match the exact current candidate",
		)
	}
	if input.Defect != ApplicationRequirementDuplicateCandidateDefect {
		return fmt.Errorf(
			"application requirement duplicate replacement defect must be %q",
			ApplicationRequirementDuplicateCandidateDefect,
		)
	}
	return nil
}

func BuildApplicationRequirementCandidateDuplicateReplacementPrompt(
	input ApplicationRequirementCandidateDuplicateReplacementInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := applicationRequirementGenerationProjection(
		input.GenerationAuthority.Authority,
	)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one complete replacement for the exact current requirement candidate below.",
		"The code-grounded defect is exact: the current candidate is byte-identical to the indexed retained value identified below. Do not reconsider that defect.",
		"Return the earliest uncovered explicit task-local runtime implementation requirement from the immutable request. It must describe exactly one independently testable runtime outcome and must not repeat or paraphrase an accepted requirement or excluded non-runtime candidate.",
		"Do not return product identity, delivery surface, language, framework, toolchain, version, packaging, tree or named-artifact constraints, generic test obligations, build or verification obligations, or deployment and continued-availability obligations.",
		"The replacement must be byte-different from the current candidate. Faithfully paraphrase only one uncovered outcome and do not add implementation detail, unstated obligations, a broader product summary, or another requirement.",
		"Return only the complete replacement requirement as raw prose. Do not return JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
		"CODE-ESTABLISHED UNCOVERED RELATION:\n" + input.GenerationAuthority.Coverage.Relation,
		"EXACT CURRENT CANDIDATE:\n" + input.CurrentCandidate,
		fmt.Sprintf(
			"CODE-GROUNDED DUPLICATE IDENTITY:\n%s at zero-based index %d",
			input.Duplicate.Set,
			input.Duplicate.Index,
		),
		"EXACT REGISTERED DEFECT:\n" + input.Defect,
		"FINAL QUESTION:\nWhat complete byte-different requirement is the earliest uncovered one-outcome runtime requirement? Return only that replacement.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(
	input ApplicationRequirementCandidateDuplicateReplacementInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeCurrentApplicationRequirementText(raw)
	if err != nil {
		return "", err
	}
	if leaf == input.CurrentCandidate {
		return "", fmt.Errorf(
			"application requirement duplicate replacement repeated the exact defective value",
		)
	}
	for _, retained := range input.GenerationAuthority.Authority.AcceptedRequirements {
		if leaf == retained {
			return "", fmt.Errorf(
				"application requirement duplicate replacement duplicates an accepted requirement",
			)
		}
	}
	for _, retained := range input.GenerationAuthority.Authority.ExcludedCandidates {
		if leaf == retained {
			return "", fmt.Errorf(
				"application requirement duplicate replacement duplicates an excluded non-runtime candidate",
			)
		}
	}
	return leaf, nil
}

// DecodeApplicationRequirementCandidateDuplicateReplacementLeafForPortableRenderer
// validates one replay response against the sole renderer that owns this kind.
func DecodeApplicationRequirementCandidateDuplicateReplacementLeafForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (string, error) {
	if renderer != PortableRendererV8 {
		return "", fmt.Errorf(
			"portable work kind %q requires renderer %q",
			WorkApplicationRequirementCandidateDuplicateReplacement,
			PortableRendererV8,
		)
	}
	var input ApplicationRequirementCandidateDuplicateReplacementInput
	if err := decodePortablePayload(payload, &input); err != nil {
		return "", err
	}
	return DecodeApplicationRequirementCandidateDuplicateReplacementLeaf(input, raw)
}
