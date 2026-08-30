package assemblyline

import (
	"fmt"
	"strings"
)

const WorkApplicationRequirementCandidateResultRelationCorrection WorkKind = "application_requirement_candidate_result_relation_correction"

type ApplicationRequirementCandidateResultRelationCorrectionInput struct {
	ImmutableRequest string                                                       `json:"immutable_request"`
	Context          ApplicationContext                                           `json:"context"`
	CurrentCandidate string                                                       `json:"current_candidate"`
	Defect           string                                                       `json:"defect"`
	Grounding        ApplicationRequirementCandidateResultRelationGroundingResult `json:"grounding"`
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
	if err := validateApplicationRequest(
		"application requirement result-relation correction", input.ImmutableRequest,
	); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext(
		"application requirement result-relation correction request",
		input.ImmutableRequest,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"application requirement result-relation correction current candidate",
		input.CurrentCandidate,
		maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if err := input.Grounding.ValidateCorrectionAuthority(
		input.ImmutableRequest,
		input.Context,
		input.CurrentCandidate,
		input.Defect,
	); err != nil {
		return err
	}
	return nil
}

func BuildApplicationRequirementCandidateResultRelationCorrectionPrompt(
	input ApplicationRequirementCandidateResultRelationCorrectionInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authorityProjection := renderApplicationContextModelProjection(
		input.ImmutableRequest,
		input.Context,
	)
	return strings.Join([]string{
		"Return one complete replacement for the exact current requirement candidate below.",
		"The exact defect is fixed: the candidate requires a derived result but does not state the semantic relation needed to determine that result independently.",
		"The supplied grounding relation is also fixed: the immutable request and established facts entail exactly one determining relation for this outcome. Express only that determining relation.",
		"Replace only this candidate with one one-outcome task-local runtime requirement grounded by the application authority. Name the determining operation or rule and its observable operands, conditions, and result meaning precisely enough for an independent test to compute the expected result.",
		"Do not add an optional feature, conventional default, implementation detail, another outcome, broader summary, test instruction, or expected example value.",
		"The replacement must be byte-different from the current candidate. Return only the complete replacement requirement as raw prose. Do not return JSON, quotes, a label, Markdown, commentary, an instruction, or a patch.",
		"APPLICATION AUTHORITY:\n" + authorityProjection,
		"EXACT CURRENT CANDIDATE:\n" + input.CurrentCandidate,
		"EXACT DEFECT:\n" + input.Defect,
		"EXACT GROUNDING RELATION:\n" + input.Grounding.Relation,
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
	if leaf == input.CurrentCandidate {
		return "", fmt.Errorf(
			"application requirement candidate result-relation correction repeated the exact defective value",
		)
	}
	return leaf, nil
}
