package assemblyline

import (
	"encoding/json"
	"fmt"
)

func validatePortableJobPayload(kind WorkKind, payload json.RawMessage) error {
	switch kind {
	case WorkApplicationContextNeeds:
		return decodeAndValidatePortablePayload[ApplicationContextNeedInput](
			payload, ApplicationContextNeedInput.validate,
		)
	case WorkApplicationIntent:
		return decodeAndValidatePortablePayload[ApplicationIntentInput](
			payload, ApplicationIntentInput.validate,
		)
	case WorkApplicationClassify:
		return decodeAndValidatePortablePayload[ApplicationClassificationInput](payload, ApplicationClassificationInput.validate)
	case WorkApplicationJobSpecification:
		return decodeAndValidatePortablePayload[ApplicationJobSpecificationInput](
			payload, validateApplicationJobSpecificationInput,
		)
	case WorkApplicationTargetTree:
		return decodeAndValidatePortablePayload[TargetTreeInput](payload, TargetTreeInput.Validate)
	case WorkApplicationAcceptanceGroundingReview:
		return decodeAndValidatePortablePayload[ApplicationAcceptanceGroundingReviewInput](
			payload, ApplicationAcceptanceGroundingReviewInput.validate,
		)
	case WorkRepositoryRequirements:
		return decodeAndValidatePortablePayload[RepositoryRequirementInterpretationInput](
			payload, RepositoryRequirementInterpretationInput.validate,
		)
	case WorkRepositorySearchTerm:
		return decodeAndValidatePortablePayload[RepositorySearchTermInput](payload, RepositorySearchTermInput.validate)
	case WorkRepositoryChangeSurface:
		return decodeAndValidatePortablePayload[RepositoryChangeSurfaceInput](payload, RepositoryChangeSurfaceInput.validate)
	case WorkRepositoryEvidenceRelevance:
		return decodeAndValidatePortablePayload[RepositoryEvidenceRelevanceInput](payload, RepositoryEvidenceRelevanceInput.validate)
	case WorkRepositoryGroundedReview:
		return decodeAndValidatePortablePayload[RepositoryGroundedReviewInput](payload, RepositoryGroundedReviewInput.validate)
	case WorkRepositoryGroundedCorrection:
		return decodeAndValidatePortablePayload[RepositoryGroundedCorrectionInput](payload, RepositoryGroundedCorrectionInput.validate)
	case WorkContextSearchTerms:
		return decodeAndValidatePortablePayload[ContextSearchTermsInput](payload, ContextSearchTermsInput.validate)
	case WorkContextRelevance:
		return decodeAndValidatePortablePayload[ContextRelevanceInput](payload, ContextRelevanceInput.validate)
	case WorkContextMinification:
		return decodeAndValidatePortablePayload[ContextMinificationInput](payload, ContextMinificationInput.validate)
	case WorkConversationObjectiveKind:
		return decodeAndValidatePortablePayload[ConversationObjectiveKindInput](payload, ConversationObjectiveKindInput.validate)
	case WorkConversationResponse:
		return decodeAndValidatePortablePayload[ConversationResponseInput](payload, ConversationResponseInput.validate)
	case WorkRoleplayGroundedResponse:
		return decodeAndValidatePortablePayload[RoleplayGroundedResponseInput](
			payload, RoleplayGroundedResponseInput.validate,
		)
	case WorkRoleplayCanonExtraction:
		return decodeAndValidatePortablePayload[RoleplayCanonExtractionInput](
			payload, RoleplayCanonExtractionInput.validate,
		)
	case WorkGroundedAnswer:
		return decodeAndValidatePortablePayload[GroundedAnswerInput](payload, GroundedAnswerInput.validate)
	case WorkDatabaseSchemaSelection:
		return decodeAndValidatePortablePayload[DatabaseSchemaSelectionInput](payload, DatabaseSchemaSelectionInput.validate)
	case WorkDatabaseQueryIntent:
		return decodeAndValidatePortablePayload[DatabaseQueryIntentInput](payload, DatabaseQueryIntentInput.validate)
	case WorkDatabaseEvidenceGap:
		return decodeAndValidatePortablePayload[DatabaseEvidenceGapInput](payload, DatabaseEvidenceGapInput.validate)
	case WorkDatabaseJoinPathSelection:
		return decodeAndValidatePortablePayload[DatabaseJoinPathSelectionInput](payload, DatabaseJoinPathSelectionInput.validate)
	case WorkWebSearchTerms:
		return decodeAndValidatePortablePayload[WebSearchTermsInput](payload, WebSearchTermsInput.validate)
	case WorkWebRelevance:
		return decodeAndValidatePortablePayload[WebRelevanceInput](payload, WebRelevanceInput.validate)
	case WorkWebGroundedSynthesis:
		return decodeAndValidatePortablePayload[WebGroundedSynthesisInput](payload, WebGroundedSynthesisInput.validate)
	case WorkWebGroundedSynthesisCorrection:
		return decodeAndValidatePortablePayload[WebGroundedSynthesisCorrectionInput](payload, WebGroundedSynthesisCorrectionInput.validate)
	case WorkWebClaimEvidenceReview:
		return decodeAndValidatePortablePayload[WebClaimEvidenceReviewInput](payload, WebClaimEvidenceReviewInput.validate)
	case WorkArtifactHandling:
		return decodeAndValidatePortablePayload[ArtifactHandlingInput](payload, ArtifactHandlingInput.validate)
	case WorkKnownArtifactTruth:
		return decodeAndValidatePortablePayload[KnownArtifactTruthInput](payload, KnownArtifactTruthInput.validate)
	case WorkDeclarationArtifactBoundary:
		return decodeAndValidatePortablePayload[DeclarationArtifactBoundaryInput](payload, DeclarationArtifactBoundaryInput.validate)
	case WorkArtifactCandidateSelection:
		return decodeAndValidatePortablePayload[ArtifactCandidateSelectionInput](payload, ArtifactCandidateSelectionInput.validate)
	case WorkCapabilityRelation:
		return decodeAndValidatePortablePayload[CapabilityRelationInput](payload, CapabilityRelationInput.validate)
	case WorkSkillSelection:
		return decodeAndValidatePortablePayload[SkillSelectionInput](payload, SkillSelectionInput.validate)
	case WorkTypeScriptRepairGuidance:
		return decodeAndValidatePortablePayload[TypeScriptRepairGuidanceInput](
			payload, TypeScriptRepairGuidanceInput.validate,
		)
	case WorkFragmentGeneration:
		return decodeAndValidatePortablePayload[FragmentGenerationInput](payload, FragmentGenerationInput.validate)
	case WorkFragmentModification:
		return decodeAndValidatePortablePayload[FragmentModificationInput](payload, FragmentModificationInput.validate)
	case WorkFragmentCorrection:
		return decodeAndValidatePortablePayload[FragmentCorrectionInput](payload, FragmentCorrectionInput.validate)
	case WorkResponseCorrection:
		return decodeAndValidatePortablePayload[ResponseCorrectionInput](payload, ResponseCorrectionInput.validate)
	default:
		return fmt.Errorf("portable job kind %q has no payload validator", kind)
	}
}

func decodeAndValidatePortablePayload[T any](payload json.RawMessage, validate func(T) error) error {
	var input T
	if err := decodePortablePayload(payload, &input); err != nil {
		return err
	}
	return validate(input)
}
