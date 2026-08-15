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
	case WorkApplicationContextNeeds:
		var input ApplicationContextNeedInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationContextNeedPrompt(input)
		return prompt, ApplicationContextNeedResponseSchema(), err
	case WorkApplicationIntent:
		var input ApplicationIntentInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationIntentPrompt(input)
		return prompt, ApplicationIntentResponseSchema(), err
	case WorkApplicationIntentReview:
		var input ApplicationIntentReviewInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationIntentReviewPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationIntentReviewResponseSchema(input)
		return prompt, schema, err
	case WorkApplicationIntentRepair:
		var input ApplicationIntentRepairInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationIntentRepairPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationIntentRepairResponseSchema(input)
		return prompt, schema, err
	case WorkApplicationClassify:
		var input ApplicationClassificationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationClassificationPrompt(input)
		return prompt, ApplicationClassificationResponseSchema(), err
	case WorkApplicationJobSpecification:
		var input ApplicationJobSpecificationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationJobSpecificationPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationJobSpecificationResponseSchema(input)
		return prompt, schema, err
	case WorkApplicationJobSpecificationReview:
		var input applicationJobSpecificationReviewPortablePayload
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderApplicationJobSpecificationReviewPortable(input)
	case WorkApplicationJobSpecificationRepair:
		var input applicationJobSpecificationRepairPortablePayload
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderApplicationJobSpecificationRepairPortable(input)
	case WorkApplicationAcceptanceGroundingReview:
		var input ApplicationAcceptanceGroundingReviewInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationAcceptanceGroundingReviewPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationAcceptanceGroundingReviewResponseSchema(input)
		return prompt, schema, err
	case WorkRepositoryRequirements:
		var input RepositoryRequirementInterpretationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositoryRequirementInterpretationPrompt(input)
		return prompt, RepositoryRequirementInterpretationResponseSchema(), err
	case WorkRepositorySearchTerm:
		var input RepositorySearchTermInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositorySearchTermPrompt(input)
		return prompt, RepositorySearchTermResponseSchema(), err
	case WorkRepositoryChangeSurface:
		var input RepositoryChangeSurfaceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositoryChangeSurfacePrompt(input)
		return prompt, RepositoryChangeSurfaceResponseSchema(input), err
	case WorkRepositoryEvidenceRelevance:
		var input RepositoryEvidenceRelevanceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositoryEvidenceRelevancePrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := RepositoryEvidenceRelevanceResponseSchema(input)
		return prompt, schema, err
	case WorkRepositoryGroundedReview:
		var input RepositoryGroundedReviewInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositoryGroundedReviewPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := RepositoryGroundedReviewResponseSchema(input)
		return prompt, schema, err
	case WorkRepositoryGroundedCorrection:
		var input RepositoryGroundedCorrectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRepositoryGroundedCorrectionPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := RepositoryGroundedCorrectionResponseSchema(input)
		return prompt, schema, err
	case WorkConversationContextSelection:
		var input ConversationContextSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildConversationContextSelectionPrompt(input)
		return prompt, ConversationContextSelectionResponseSchema(), err
	case WorkMemoryContextSelection:
		var input MemoryContextSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildMemoryContextSelectionPrompt(input)
		return prompt, MemoryContextSelectionResponseSchema(), err
	case WorkConversationObjectiveKind:
		var input ConversationObjectiveKindInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildConversationObjectiveKindPrompt(input)
		return prompt, ConversationObjectiveKindResponseSchema(), err
	case WorkConversationResponse:
		var input ConversationResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildConversationResponsePrompt(input)
		return prompt, ConversationResponseSchema(), err
	case WorkGroundedAnswer:
		var input GroundedAnswerInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildGroundedAnswerPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := GroundedAnswerResponseSchema(input)
		return prompt, schema, err
	case WorkWebSearchTerms:
		var input WebSearchTermsInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildWebSearchTermsPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := WebSearchTermsResponseSchema(input)
		return prompt, schema, err
	case WorkWebRelevance:
		var input WebRelevanceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildWebRelevancePrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := WebRelevanceResponseSchema(input)
		return prompt, schema, err
	case WorkWebGroundedSynthesis:
		var input WebGroundedSynthesisInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildWebGroundedSynthesisPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := WebGroundedSynthesisResponseSchema(input)
		return prompt, schema, err
	case WorkWebGroundedSynthesisCorrection:
		var input WebGroundedSynthesisCorrectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildWebGroundedSynthesisCorrectionPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := WebGroundedSynthesisCorrectionResponseSchema(input)
		return prompt, schema, err
	case WorkWebClaimEvidenceReview:
		var input WebClaimEvidenceReviewInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildWebClaimEvidenceReviewPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := WebClaimEvidenceReviewResponseSchema(input)
		return prompt, schema, err
	case WorkArtifactHandling:
		var input ArtifactHandlingInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildArtifactHandlingPrompt(input)
		return prompt, ArtifactHandlingResponseSchema(input.Token), err
	case WorkKnownArtifactTruth:
		var input KnownArtifactTruthInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildKnownArtifactTruthPrompt(input)
		return prompt, KnownArtifactTruthResponseSchema(), err
	case WorkDeclarationArtifactBoundary:
		var input DeclarationArtifactBoundaryInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildDeclarationArtifactBoundaryPrompt(input)
		return prompt, DeclarationArtifactBoundaryResponseSchema(input), err
	case WorkArtifactCandidateSelection:
		var input ArtifactCandidateSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildArtifactCandidateSelectionPrompt(input)
		return prompt, ArtifactCandidateSelectionResponseSchema(input), err
	case WorkCapabilityRelation:
		var input CapabilityRelationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildCapabilityRelationPrompt(input)
		return prompt, CapabilityRelationResponseSchema(), err
	case WorkSkillSelection:
		var input SkillSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildSkillSelectionPrompt(input)
		return prompt, SkillSelectionResponseSchema(input), err
	case WorkTypeScriptRepairGuidance:
		var input TypeScriptRepairGuidanceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildTypeScriptRepairGuidancePrompt(input)
		return prompt, TypeScriptRepairGuidanceResponseSchema(), err
	case WorkFragmentGeneration:
		var input FragmentGenerationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		return renderPortableFragmentGeneration(input)
	case WorkFragmentModification:
		var input FragmentModificationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildGoFragmentModificationPrompt(input)
		return prompt, nil, err
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
	switch input.Language {
	case "go":
		prompt, err := BuildGoFragmentGenerationPrompt(input)
		return prompt, nil, err
	case "typescript":
		prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
			Signature: input.Signature,
			Contract:  input.Behavior,
			Available: strings.Join(input.Capabilities, "\n"),
			Globals:   input.PermittedSymbols,
		})
		return prompt, nil, err
	default:
		return "", nil, fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, map[string]any, error) {
	switch input.Language {
	case "go":
		prompt, err := BuildGoFragmentCorrectionPrompt(input)
		return prompt, nil, err
	case "typescript":
		prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
			Signature:      input.Signature,
			Available:      strings.Join(input.Capabilities, "\n"),
			Globals:        input.PermittedSymbols,
			Current:        input.CurrentDeclaration,
			RepairRegion:   input.RepairRegion,
			RequiredChange: input.RequiredChange,
			Diagnostic:     input.Diagnostic,
			RepairGuidance: input.RepairGuidance,
		})
		return prompt, nil, err
	default:
		return "", nil, fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
}

func renderPortableResponseCorrection(input ResponseCorrectionInput) (string, map[string]any, error) {
	schema, err := responseCorrectionSchema(input.Original, input.TargetField)
	if err != nil {
		return "", nil, err
	}
	if input.Original.Kind == WorkApplicationJobSpecification {
		prompt, promptErr := buildApplicationJobSpecificationResponseCorrectionPrompt(input)
		return prompt, schema, promptErr
	}
	instruction := "Return a JSON merge patch containing exactly one top-level field and changing exactly one invalid leaf. " +
		"The retained response and its accepted fields are code-owned and unavailable. Resolve only this failure:\n" +
		input.ValidationFailure
	return instruction, schema, nil
}
