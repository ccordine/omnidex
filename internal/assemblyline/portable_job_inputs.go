package assemblyline

import (
	"fmt"
	"strings"
)

type ApplicationClassificationInput struct {
	UserRequest string `json:"user_request"`
}

type ArtifactHandlingInput struct {
	UserRequest string `json:"user_request"`
	Token       string `json:"token"`
}

type FragmentGenerationInput struct {
	Language         string   `json:"language"`
	Dialect          string   `json:"dialect"`
	Signature        string   `json:"signature"`
	Behavior         string   `json:"behavior"`
	Capabilities     []string `json:"capabilities"`
	PermittedSymbols []string `json:"permitted_symbols"`
}

type FragmentCorrectionInput struct {
	Language           string                          `json:"language,omitempty"`
	Signature          string                          `json:"signature,omitempty"`
	Capabilities       []string                        `json:"capabilities,omitempty"`
	PermittedSymbols   []string                        `json:"permitted_symbols,omitempty"`
	CurrentDeclaration string                          `json:"current_declaration,omitempty"`
	RepairRegion       *TypeScriptFragmentRepairRegion `json:"repair_region,omitempty"`
	RequiredChange     string                          `json:"required_change,omitempty"`
	Diagnostic         string                          `json:"diagnostic,omitempty"`
	RepairGuidance     string                          `json:"repair_guidance,omitempty"`
}

type ResponseCorrectionInput struct {
	Original          PortableJob `json:"original"`
	ValidationFailure string      `json:"validation_failure"`
	TargetField       string      `json:"target_field,omitempty"`
	RetainedCandidate string      `json:"retained_candidate,omitempty"`
}

func NewApplicationContextNeedJob(input ApplicationContextNeedInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationContextNeeds, input, input.validate)
}

func NewApplicationIntentJob(input ApplicationIntentInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationIntent, input, input.validate)
}

func NewApplicationClassificationJob(input ApplicationClassificationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationClassify, input, input.validate)
}

func NewApplicationJobSpecificationJob(input ApplicationJobSpecificationInput) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationJobSpecification, input,
		func() error { return validateApplicationJobSpecificationInput(input) },
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
	if input.Language != "typescript" {
		return PortableJob{}, fmt.Errorf(
			"language-blind fragment correction requires the source-projected constructor",
		)
	}
	return newValidatedPortableJob(WorkFragmentCorrection, input, input.validate)
}

// NewSourceProjectedFragmentCorrectionJob binds a language-blind correction
// prompt to one code-owned result decoder. SourceProjection is immutable
// portable evidence but is never rendered into the model-visible prompt.
func NewSourceProjectedFragmentCorrectionJob(
	input FragmentCorrectionInput,
	sourceProjection string,
) (PortableJob, error) {
	if sourceProjection == "" || sourceProjection != strings.TrimSpace(sourceProjection) {
		return PortableJob{}, fmt.Errorf(
			"source-projected fragment correction requires one trimmed projection identity",
		)
	}
	job, err := newValidatedPortableJob(WorkFragmentCorrection, input, input.validate)
	if err != nil {
		return PortableJob{}, err
	}
	job.SourceProjection = sourceProjection
	job.ID = portableJobProjectionDigest(
		job.Schema, job.Kind, job.Payload, job.SourceProjection,
	)
	if err := job.Validate(); err != nil {
		return PortableJob{}, err
	}
	return job, nil
}

func NewResponseCorrectionJob(
	original PortableJob,
	validationFailure string,
) (PortableJob, error) {
	return PortableJob{}, fmt.Errorf(
		"response correction requires the exact retained candidate; use NewRetainedResponseCorrectionJob",
	)
}

func NewRetainedResponseCorrectionJob(
	original PortableJob,
	validationFailure string,
	retainedCandidate string,
) (PortableJob, error) {
	input := ResponseCorrectionInput{
		Original: original, ValidationFailure: strings.TrimSpace(validationFailure),
		RetainedCandidate: strings.TrimSpace(retainedCandidate),
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
