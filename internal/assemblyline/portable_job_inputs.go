package assemblyline

import "strings"

type RequirementPartitionMode string

const (
	RequirementExtractFeatures RequirementPartitionMode = "extract_features"
	RequirementSplitFeature    RequirementPartitionMode = "split_feature"
)

type RequirementPartitionInput struct {
	SourceText string                   `json:"source_text"`
	Mode       RequirementPartitionMode `json:"mode"`
}

type ApplicationClassificationInput struct {
	UserRequest string `json:"user_request"`
}

type ApplicationIdentityInput struct {
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
	Language           string   `json:"language"`
	Signature          string   `json:"signature"`
	Capabilities       []string `json:"capabilities"`
	PermittedSymbols   []string `json:"permitted_symbols"`
	CurrentDeclaration string   `json:"current_declaration"`
	RequiredChange     string   `json:"required_change"`
	Diagnostic         string   `json:"diagnostic"`
}

type ResponseCorrectionInput struct {
	Original          PortableJob `json:"original"`
	ValidationFailure string      `json:"validation_failure"`
}

func NewRequirementPartitionJob(input RequirementPartitionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkRequirementPartition, input, input.validate)
}

func NewApplicationClassificationJob(input ApplicationClassificationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationClassify, input, input.validate)
}

func NewApplicationIdentityJob(input ApplicationIdentityInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationIdentity, input, input.validate)
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

func newValidatedPortableJob(kind WorkKind, input any, validate func() error) (PortableJob, error) {
	if err := validate(); err != nil {
		return PortableJob{}, err
	}
	return newPortableJob(kind, input)
}
