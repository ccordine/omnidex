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
	Language                 string                            `json:"language"`
	Dialect                  string                            `json:"dialect"`
	Signature                string                            `json:"signature"`
	Behavior                 string                            `json:"behavior"`
	Capabilities             []string                          `json:"capabilities"`
	PermittedSymbols         []string                          `json:"permitted_symbols"`
	PublicInteractionSurface *FragmentPublicInteractionSurface `json:"public_interaction_surface,omitempty"`
}

// FragmentGenerationReplacementInput preserves only the unresolved source
// responsibility. Code-owned relational journal state proves why the bounded
// replacement is legal and deliberately remains outside portable identity.
type FragmentGenerationReplacementInput struct {
	Original FragmentGenerationInput `json:"original"`
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

func NewApplicationClassificationJob(input ApplicationClassificationInput) (PortableJob, error) {
	return newPortableJob(WorkApplicationClassify, input)
}

func NewArtifactHandlingJob(input ArtifactHandlingInput) (PortableJob, error) {
	return newPortableJob(WorkArtifactHandling, input)
}

func NewCapabilityRelationJob(input CapabilityRelationInput) (PortableJob, error) {
	return newPortableJob(WorkCapabilityRelation, input)
}

func NewFragmentGenerationJob(input FragmentGenerationInput) (PortableJob, error) {
	return newPortableJob(WorkFragmentGeneration, input)
}

func NewFragmentGenerationReplacementJob(
	input FragmentGenerationReplacementInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkFragmentGenerationReplacement, input,
	)
}

func NewFragmentCorrectionJob(input FragmentCorrectionInput) (PortableJob, error) {
	if input.Language != "typescript" {
		return PortableJob{}, fmt.Errorf(
			"language-blind fragment correction requires the source-projected constructor",
		)
	}
	return newPortableJob(WorkFragmentCorrection, input)
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
	job, err := newPortableJob(WorkFragmentCorrection, input)
	if err != nil {
		return PortableJob{}, err
	}
	job.SourceProjection = sourceProjection
	job.ID = portableJobProjectionDigest(
		job.Schema, job.Kind, job.Payload, job.SourceProjection,
	)
	if err := validatePortableJobSourceProjection(job); err != nil {
		return PortableJob{}, err
	}
	return job, nil
}
