package assemblyline

import (
	"fmt"
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
	case WorkApplicationProjectStackConstraint:
		var input ApplicationProjectStackConstraintInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationProjectStackConstraintPrompt(input)
		return prompt, ApplicationProjectStackConstraintResponseSchema(input), err
	case WorkApplicationServiceContinuedAvailability:
		var input ApplicationServiceContinuedAvailabilityInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceContinuedAvailabilityPrompt(input)
		return prompt, ApplicationServiceContinuedAvailabilityResponseSchema(), err
	case WorkApplicationServicePersistenceDestination:
		var input ApplicationServicePersistenceDestinationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServicePersistenceDestinationPrompt(input)
		return prompt, ApplicationServicePersistenceDestinationResponseSchema(), err
	case WorkApplicationServiceStateLifetime:
		var input ApplicationServiceStateLifetimeInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceStateLifetimePrompt(input)
		return prompt, ApplicationServiceStateLifetimeResponseSchema(), err
	case WorkApplicationServiceStateInterface:
		var input ApplicationServiceStateInterfaceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceStateInterfacePrompt(input)
		return prompt, ApplicationServiceStateInterfaceResponseSchema(), err
	case WorkApplicationServiceEndpointRequirement:
		var input ApplicationServiceEndpointRequirementInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointRequirementPrompt(input)
		return prompt, ApplicationServiceEndpointRequirementResponseSchema(), err
	case WorkApplicationServiceEndpointExposure:
		var input ApplicationServiceEndpointExposureInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointExposurePrompt(input)
		return prompt, ApplicationServiceEndpointExposureResponseSchema(), err
	case WorkApplicationServiceEndpointMethod:
		var input ApplicationServiceEndpointMethodInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointMethodPrompt(input)
		return prompt, ApplicationServiceEndpointMethodResponseSchema(), err
	case WorkApplicationServiceEndpointRouteTemplate:
		var input ApplicationServiceEndpointRouteTemplateInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointRouteTemplatePrompt(input)
		return prompt, ApplicationServiceEndpointRouteTemplateResponseSchema(), err
	case WorkApplicationServiceEndpointRequestMedia:
		var input ApplicationServiceEndpointRequestMediaInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointRequestMediaPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationServiceEndpointRequestMediaResponseSchema(input)
		return prompt, schema, err
	case WorkApplicationServiceEndpointResponseMedia:
		var input ApplicationServiceEndpointResponseMediaInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointResponseMediaPrompt(input)
		return prompt, ApplicationServiceEndpointResponseMediaResponseSchema(), err
	case WorkApplicationServiceEndpointSuccessStatus:
		var input ApplicationServiceEndpointSuccessStatusInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildApplicationServiceEndpointSuccessStatusPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ApplicationServiceEndpointSuccessStatusResponseSchema(input)
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
	case WorkApplicationTargetTree:
		var input TargetTreeInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildTargetTreePrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := TargetTreeResponseSchema(input)
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
	case WorkContextSearchTerms:
		var input ContextSearchTermsInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildContextSearchTermsPrompt(input)
		return prompt, ContextSearchTermsResponseSchema(), err
	case WorkContextRelevance:
		var input ContextRelevanceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildContextRelevancePrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := ContextRelevanceResponseSchema(input)
		return prompt, schema, err
	case WorkContextMinification:
		var input ContextMinificationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildContextMinificationPrompt(input)
		return prompt, ContextMinificationResponseSchema(), err
	case WorkConversationObjectiveKind:
		var input ConversationObjectiveKindInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildConversationObjectiveKindPrompt(input)
		return prompt, ConversationObjectiveKindResponseSchema(input), err
	case WorkConversationResponse:
		var input ConversationResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildConversationResponsePrompt(input)
		return prompt, ConversationResponseSchema(), err
	case WorkRoleplayGroundedResponse:
		var input RoleplayGroundedResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRoleplayGroundedResponsePrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := RoleplayGroundedResponseSchema(input)
		return prompt, schema, err
	case WorkRoleplayCanonExtraction:
		var input RoleplayCanonExtractionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRoleplayCanonExtractionPrompt(input)
		return prompt, RoleplayCanonExtractionResponseSchema(), err
	case WorkRoleplayOngoingAction:
		var input RoleplayOngoingActionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildRoleplayOngoingActionPrompt(input)
		return prompt, RoleplayOngoingActionResponseSchema(), err
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
	case WorkDatabaseSchemaSelection:
		var input DatabaseSchemaSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildDatabaseSchemaSelectionPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := DatabaseSchemaSelectionResponseSchema(input)
		return prompt, schema, err
	case WorkDatabaseQueryIntent:
		var input DatabaseQueryIntentInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildDatabaseQueryIntentPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := DatabaseQueryIntentResponseSchema(input)
		return prompt, schema, err
	case WorkDatabaseEvidenceGap:
		var input DatabaseEvidenceGapInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildDatabaseEvidenceGapPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := DatabaseEvidenceGapResponseSchema(input)
		return prompt, schema, err
	case WorkDatabaseJoinPathSelection:
		var input DatabaseJoinPathSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildDatabaseJoinPathSelectionPrompt(input)
		if err != nil {
			return "", nil, err
		}
		schema, err := DatabaseJoinPathSelectionResponseSchema(input)
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
		var input FragmentRepairGuidanceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return "", nil, err
		}
		prompt, err := BuildFragmentRepairGuidancePrompt(input)
		return prompt, FragmentRepairGuidanceResponseSchema(), err
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
