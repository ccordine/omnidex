package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkApplicationRequirementCandidateSplitCorrection WorkKind = "application_requirement_candidate_split_correction"
	ApplicationRequirementUnchangedSplitDefect                  = "UNCHANGED_MULTI_OUTCOME_CANDIDATE"
)

type ApplicationRequirementCandidateSplitCorrectionInput struct {
	CurrentCandidate string                                           `json:"current_candidate"`
	Cardinality      ApplicationRequirementCandidateCardinalityResult `json:"cardinality"`
	Defect           string                                           `json:"defect"`
}

func NewApplicationRequirementCandidateSplitCorrectionJob(
	input ApplicationRequirementCandidateSplitCorrectionInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateSplitCorrection, input, input.validate,
	)
}

func (input ApplicationRequirementCandidateSplitCorrectionInput) validate() error {
	cardinalityInput := ApplicationRequirementCandidateCardinalityInput{
		Candidate: input.CurrentCandidate,
	}
	if err := input.Cardinality.ValidateFor(cardinalityInput); err != nil {
		return fmt.Errorf("validate application requirement split correction cardinality: %w", err)
	}
	if input.Cardinality.Relation != ApplicationRequirementMultipleRuntimeOutcomes {
		return fmt.Errorf(
			"application requirement split correction requires code-established relation %q",
			ApplicationRequirementMultipleRuntimeOutcomes,
		)
	}
	if input.Defect != ApplicationRequirementUnchangedSplitDefect {
		return fmt.Errorf(
			"application requirement split correction defect must be %q",
			ApplicationRequirementUnchangedSplitDefect,
		)
	}
	return nil
}

func BuildApplicationRequirementCandidateSplitCorrectionPrompt(
	input ApplicationRequirementCandidateSplitCorrectionInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return one complete replacement for the exact current requirement candidate below.",
		"The grounded defect is exact: the prior split returned these same bytes even though the code-bound cardinality proves that they contain multiple independently testable runtime outcomes.",
		"Replace the current value with only its earliest independently testable runtime outcome. Omit every later action, element, quality, or outcome. The replacement must be byte-different and require only one independent runtime assertion.",
		"Do not add context, implementation detail, another outcome, a broader summary, or information not present in the current candidate.",
		"Return only the complete replacement requirement as raw prose. Do not return JSON, quotes, a label, Markdown, commentary, an instruction, or a patch.",
		"EXACT CURRENT CANDIDATE:\n" + input.CurrentCandidate,
		"CODE-ESTABLISHED CARDINALITY RELATION:\n" + input.Cardinality.Relation,
		"EXACT GROUNDED DEFECT:\nThe prior returned value was byte-identical to the exact current candidate.",
		"FINAL QUESTION:\nWhat complete byte-different replacement retains only the earliest one-outcome requirement? Return only that replacement.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateSplitCorrectionLeaf(
	input ApplicationRequirementCandidateSplitCorrectionInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate split correction", raw,
		maxRequirementQuoteBytes, true,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationIntentText(
		"application requirement candidate split correction", leaf,
		maxRequirementQuoteBytes,
	); err != nil {
		return "", err
	}
	if leaf == input.CurrentCandidate {
		return "", fmt.Errorf("application requirement candidate split correction repeated the exact defective value")
	}
	return leaf, nil
}
