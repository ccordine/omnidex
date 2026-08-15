package assemblyline

import "strings"

type ApplicationClassificationInput struct {
	UserRequest string `json:"user_request"`
}

type ArtifactHandlingInput struct {
	UserRequest string `json:"user_request"`
	Token       string `json:"token"`
}

type FragmentGenerationInput struct {
	Language         string   `json:"language"`
	Signature        string   `json:"signature"`
	Behavior         string   `json:"behavior"`
	Capabilities     []string `json:"capabilities"`
	PermittedSymbols []string `json:"permitted_symbols"`
}

type FragmentCorrectionInput struct {
	Language           string                          `json:"language"`
	Signature          string                          `json:"signature"`
	Capabilities       []string                        `json:"capabilities"`
	PermittedSymbols   []string                        `json:"permitted_symbols"`
	CurrentDeclaration string                          `json:"current_declaration,omitempty"`
	RepairRegion       *TypeScriptFragmentRepairRegion `json:"repair_region,omitempty"`
	RequiredChange     string                          `json:"required_change"`
	Diagnostic         string                          `json:"diagnostic"`
}

type ResponseCorrectionInput struct {
	Original          PortableJob `json:"original"`
	ValidationFailure string      `json:"validation_failure"`
	TargetField       string      `json:"target_field,omitempty"`
}

func NewApplicationClassificationJob(input ApplicationClassificationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationClassify, input, input.validate)
}

func NewApplicationRequirementInterpretationJob(
	input ApplicationRequirementInterpretationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRequirements, input, input.validate)
}

func NewApplicationJobSpecificationJob(input ApplicationJobSpecificationInput) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationJobSpecification, input,
		func() error { return validateApplicationJobSpecificationInput(input) },
	)
}

func NewApplicationJobSpecificationReviewJob(
	input ApplicationJobSpecificationReviewInput,
) (PortableJob, error) {
	payload, err := newApplicationJobSpecificationReviewPortablePayload(input)
	if err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationJobSpecificationReview, payload)
}

func NewApplicationJobSpecificationRepairJob(
	input ApplicationJobSpecificationRepairInput,
) (PortableJob, error) {
	payload, err := newApplicationJobSpecificationRepairPortablePayload(input)
	if err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(WorkApplicationJobSpecificationRepair, payload)
}

func NewApplicationAcceptanceGroundingReviewJob(
	input ApplicationAcceptanceGroundingReviewInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationAcceptanceGroundingReview, input, input.validate,
	)
}

func NewRepositoryRequirementInterpretationJob(
	input RepositoryRequirementInterpretationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkRepositoryRequirements, input, input.validate)
}

func NewArtifactHandlingJob(input ArtifactHandlingInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkArtifactHandling, input, input.validate)
}

func NewCapabilityRelationJob(input CapabilityRelationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkCapabilityRelation, input, input.validate)
}

func NewSkillSelectionJob(input SkillSelectionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkSkillSelection, input, input.validate)
}

func NewFragmentGenerationJob(input FragmentGenerationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkFragmentGeneration, input, input.validate)
}

func NewFragmentCorrectionJob(input FragmentCorrectionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkFragmentCorrection, input, input.validate)
}

func NewResponseCorrectionJob(
	original PortableJob,
	validationFailure string,
) (PortableJob, error) {
	input := ResponseCorrectionInput{
		Original: original, ValidationFailure: strings.TrimSpace(validationFailure),
	}
	return newValidatedPortableJob(WorkResponseCorrection, input, input.validate)
}

func NewResponseCorrectionJobForField(
	original PortableJob,
	validationFailure string,
	targetField string,
) (PortableJob, error) {
	input := ResponseCorrectionInput{
		Original: original, ValidationFailure: strings.TrimSpace(validationFailure),
		TargetField: targetField,
	}
	return newValidatedPortableJob(WorkResponseCorrection, input, input.validate)
}

func newValidatedPortableJob(kind WorkKind, input any, validate func() error) (PortableJob, error) {
	if err := validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(kind, input)
}
