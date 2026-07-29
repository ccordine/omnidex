package assemblyline

import (
	"fmt"
	"strings"
)

// RenderPortableJob is the sole mapping from an immutable work envelope to
// model-visible context. Schedulers may choose a model or machine, but cannot
// add workspace state or instructions to the job.
func RenderPortableJob(job PortableJob) (string, map[string]any, error) {
	if err := job.Validate(); err != nil {
		return "", nil, err
	}
	switch job.Kind {
	case WorkApplicationClassify:
		var input ApplicationClassificationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationClassificationPrompt(input)
		return prompt, ApplicationClassificationResponseSchema(), err
	case WorkApplicationIdentity:
		var input ApplicationIdentityInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationIdentityPrompt(input)
		return prompt, ApplicationIdentityResponseSchema(), err
	case WorkRequirementPartition:
		var input RequirementPartitionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRequirementPartitionPrompt(input)
		return prompt, RequirementPartitionResponseSchema(), err
	case WorkArtifactHandling:
		var input ArtifactHandlingInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildArtifactHandlingPrompt(input)
		return prompt, ArtifactHandlingResponseSchema(input.Token), err
	case WorkCapabilityRelation:
		var input CapabilityRelationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildCapabilityRelationPrompt(input)
		return prompt, CapabilityRelationResponseSchema(), err
	case WorkSkillProcedure:
		var input SkillProcedureInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildSkillProcedurePrompt(input)
		return prompt, SkillProcedureResponseSchema(), err
	case WorkSkillSelection:
		var input SkillSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildSkillSelectionPrompt(input)
		return prompt, SkillSelectionResponseSchema(input), err
	case WorkFragmentGeneration:
		var input FragmentGenerationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderPortableFragmentGeneration(input)
	case WorkFragmentCorrection:
		var input FragmentCorrectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderPortableFragmentCorrection(input)
	case WorkResponseCorrection:
		var input ResponseCorrectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderPortableResponseCorrection(input)
	default:
		return "", nil, fmt.Errorf("portable job kind %q has no renderer", job.Kind)
	}
}

func renderPortableFragmentGeneration(input FragmentGenerationInput) (string, map[string]any, error) {
	if input.Language != "typescript" {
		return "", nil, fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature: input.Signature,
		Contract:  input.Behavior,
		Available: strings.Join(input.Capabilities, "\n"),
		Globals:   input.PermittedSymbols,
	})
	return prompt, nil, err
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, map[string]any, error) {
	if input.Language != "typescript" {
		return "", nil, fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
	prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
		Signature:      input.Signature,
		Available:      strings.Join(input.Capabilities, "\n"),
		Globals:        input.PermittedSymbols,
		Current:        input.CurrentDeclaration,
		RequiredChange: input.RequiredChange,
		Diagnostic:     input.Diagnostic,
	})
	return prompt, nil, err
}

func renderPortableResponseCorrection(input ResponseCorrectionInput) (string, map[string]any, error) {
	schema, err := responseCorrectionSchema(input.Original)
	if err != nil {
		return "", nil, err
	}
	return "Return a JSON merge patch containing exactly one top-level field and changing exactly one invalid leaf. " +
		"The retained response and its accepted fields are code-owned and unavailable. Resolve only this failure:\n" +
		input.ValidationFailure, schema, nil
}
