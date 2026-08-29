package assemblyline

import (
	"fmt"
	"strings"
)

const WorkApplicationRequirementCandidateResultRelationCorrection WorkKind = "application_requirement_candidate_result_relation_correction"

type ApplicationRequirementCandidateResultRelationCorrectionInput struct {
	GenerationAuthority ApplicationRequirementCandidateInput                `json:"generation_authority"`
	CandidateAuthority  ApplicationRequirementCandidateResultRelationInput  `json:"candidate_authority"`
	ResultRelation      ApplicationRequirementCandidateResultRelationResult `json:"result_relation"`
}

func NewApplicationRequirementCandidateResultRelationCorrectionJob(
	input ApplicationRequirementCandidateResultRelationCorrectionInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCandidateResultRelationCorrection,
		input,
		input.validate,
	)
}

func (input ApplicationRequirementCandidateResultRelationCorrectionInput) validate() error {
	if err := input.GenerationAuthority.validate(); err != nil {
		return fmt.Errorf("validate result-relation correction generation authority: %w", err)
	}
	if err := input.CandidateAuthority.validate(); err != nil {
		return fmt.Errorf("validate result-relation correction candidate authority: %w", err)
	}
	if err := input.ResultRelation.ValidateFor(input.CandidateAuthority); err != nil {
		return fmt.Errorf("validate result-relation correction receipt: %w", err)
	}
	if input.ResultRelation.Relation != ApplicationRequirementMissingResultRelation {
		return fmt.Errorf(
			"application requirement result-relation correction requires code-established relation %q",
			ApplicationRequirementMissingResultRelation,
		)
	}
	return nil
}

func BuildApplicationRequirementCandidateResultRelationCorrectionPrompt(
	input ApplicationRequirementCandidateResultRelationCorrectionInput,
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
		"The code-bound semantic relation establishes one exact defect: the candidate requires a derived result but does not state the semantic relation needed to determine that result independently.",
		"Replace only this candidate with the earliest one-outcome task-local runtime requirement grounded by the immutable request. Name one determining operation or rule and its observable operands, conditions, and result meaning precisely enough for an independent test to compute the expected result.",
		"Use an otherwise unstated relation only when the immutable request semantically entails exactly one determining rule and its operands, conditions, and result meaning for this outcome; ambiguity means the relation is missing. Do not add an optional feature, implementation detail, another outcome, broader summary, test instruction, or expected example value. Do not repeat an accepted or excluded candidate.",
		"The replacement must be byte-different from the current candidate. Return only the complete replacement requirement as raw prose. Do not return JSON, quotes, a label, Markdown, commentary, an instruction, or a patch.",
		"APPLICATION REQUIREMENT INPUT:\n" + projection,
		"EXACT CURRENT CANDIDATE:\n" + input.CandidateAuthority.Candidate,
		"CODE-ESTABLISHED RESULT RELATION:\n" + input.ResultRelation.Relation,
		"FINAL QUESTION:\nWhat complete byte-different one-outcome replacement states the missing determining relation? Return only that replacement.",
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf(
	input ApplicationRequirementCandidateResultRelationCorrectionInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate result-relation correction",
		raw,
		maxRequirementQuoteBytes,
		true,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationIntentText(
		"application requirement candidate result-relation correction",
		leaf,
		maxRequirementQuoteBytes,
	); err != nil {
		return "", err
	}
	if leaf == input.CandidateAuthority.Candidate {
		return "", fmt.Errorf(
			"application requirement candidate result-relation correction repeated the exact defective value",
		)
	}
	for _, accepted := range input.GenerationAuthority.Authority.AcceptedRequirements {
		if leaf == accepted {
			return "", fmt.Errorf(
				"application requirement candidate result-relation correction duplicated an accepted requirement",
			)
		}
	}
	for _, excluded := range input.GenerationAuthority.Authority.ExcludedCandidates {
		if leaf == excluded {
			return "", fmt.Errorf(
				"application requirement candidate result-relation correction duplicated an excluded candidate",
			)
		}
	}
	return leaf, nil
}
