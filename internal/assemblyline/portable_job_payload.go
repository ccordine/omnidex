package assemblyline

import (
	"encoding/json"
	"fmt"
)

func validatePortableJobPayload(kind WorkKind, payload json.RawMessage) error {
	switch kind {
	case WorkApplicationProductContext:
		return decodeAndValidatePortablePayload[ApplicationProductContextInput](
			payload, ApplicationProductContextInput.validate,
		)
	case WorkApplicationRequirementInventory:
		return decodeAndValidatePortablePayload[ApplicationRequirementInventoryInput](
			payload, ApplicationRequirementInventoryInput.validate,
		)
	case WorkApplicationRequirementCandidateCardinality:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateCardinalityInput](
			payload, ApplicationRequirementCandidateCardinalityInput.validate,
		)
	case WorkApplicationRequirementCandidateKind:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateContentPresenceInput](
			payload, ApplicationRequirementCandidateContentPresenceInput.validate,
		)
	case WorkApplicationRequirementCandidateAuthorization:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateAuthorizationInput](
			payload, ApplicationRequirementCandidateAuthorizationInput.validate,
		)
	case WorkApplicationRequirementCandidateOutcomeRelation:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateOutcomeRelationInput](
			payload, ApplicationRequirementCandidateOutcomeRelationInput.validateForModel,
		)
	case WorkApplicationRequirementCandidatePartition:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidatePartitionInput](
			payload, ApplicationRequirementCandidatePartitionInput.validate,
		)
	case WorkApplicationContextQuestionInventory:
		return decodeAndValidatePortablePayload[ApplicationContextQuestionInventoryInput](
			payload, ApplicationContextQuestionInventoryInput.validate,
		)
	case WorkApplicationContextQuestionNecessity:
		return decodeAndValidatePortablePayload[ApplicationContextQuestionNecessityInput](
			payload, ApplicationContextQuestionNecessityInput.validate,
		)
	case WorkApplicationContextQuestionRelation:
		return decodeAndValidatePortablePayload[ApplicationContextQuestionRelationInput](
			payload, ApplicationContextQuestionRelationInput.validate,
		)
	case WorkApplicationProjectStackConstraint:
		return decodeAndValidatePortablePayload[ApplicationProjectStackConstraintInput](
			payload, ApplicationProjectStackConstraintInput.validate,
		)
	case WorkApplicationClassify:
		return decodeAndValidatePortablePayload[ApplicationClassificationInput](payload, ApplicationClassificationInput.validate)
	case WorkRepositoryRequirementInventory:
		return decodeAndValidatePortablePayload[RepositoryRequirementInterpretationInput](
			payload, RepositoryRequirementInterpretationInput.validate,
		)
	case WorkRepositoryRequirementCandidateAuthorization:
		return decodeAndValidatePortablePayload[RepositoryRequirementCandidateAuthorizationInput](
			payload, RepositoryRequirementCandidateAuthorizationInput.validate,
		)
	case WorkRepositoryRequirementCandidateRelation:
		return decodeAndValidatePortablePayload[RepositoryRequirementCandidateRelationInput](
			payload, RepositoryRequirementCandidateRelationInput.validate,
		)
	case WorkContextRelevanceRelation:
		return decodeAndValidatePortablePayload[ContextRelevanceRelationInput](
			payload, ContextRelevanceRelationInput.validate,
		)
	case WorkContextMinification:
		return decodeAndValidatePortablePayload[ContextMinificationInput](payload, ContextMinificationInput.validate)
	case WorkConversationObjectiveKind:
		return decodeAndValidatePortablePayload[ConversationObjectiveKindInput](payload, ConversationObjectiveKindInput.validate)
	case WorkConversationResponse:
		return decodeAndValidatePortablePayload[ConversationResponseInput](payload, ConversationResponseInput.validate)
	case WorkRoleplayGroundedResponseParagraphInventory:
		return decodeAndValidatePortablePayload[RoleplayGroundedResponseInput](
			payload, RoleplayGroundedResponseInput.validate,
		)
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return decodeAndValidatePortablePayload[RoleplayGroundedEvidenceRelationInput](
			payload, RoleplayGroundedEvidenceRelationInput.validate,
		)
	case WorkRoleplayGroundedResponseParagraphAuthorization:
		return decodeAndValidatePortablePayload[RoleplayGroundedParagraphAuthorizationInput](
			payload, RoleplayGroundedParagraphAuthorizationInput.validate,
		)
	case WorkRoleplayCanonFactInventory:
		return decodeAndValidatePortablePayload[RoleplayCanonExtractionInput](
			payload, RoleplayCanonExtractionInput.validate,
		)
	case WorkRoleplayCanonFactCandidateAuthorization:
		return decodeAndValidatePortablePayload[RoleplayCanonFactCandidateAuthorizationInput](
			payload, RoleplayCanonFactCandidateAuthorizationInput.validate,
		)
	case WorkRoleplayCanonFactCandidateRelation:
		return decodeAndValidatePortablePayload[RoleplayCanonFactCandidateRelationInput](
			payload, RoleplayCanonFactCandidateRelationInput.validate,
		)
	case WorkRoleplayOngoingAction:
		return decodeAndValidatePortablePayload[RoleplayOngoingActionInput](
			payload, RoleplayOngoingActionInput.validate,
		)
	case WorkGroundedAnswerParagraphInventory:
		return decodeAndValidatePortablePayload[GroundedAnswerParagraphInventoryInput](
			payload, GroundedAnswerParagraphInventoryInput.validate,
		)
	case WorkGroundedAnswerParagraphEvidenceRelation:
		return decodeAndValidatePortablePayload[GroundedAnswerParagraphEvidenceRelationInput](
			payload, GroundedAnswerParagraphEvidenceRelationInput.validate,
		)
	case WorkGroundedAnswerParagraphAuthorization:
		return decodeAndValidatePortablePayload[GroundedAnswerParagraphAuthorizationInput](
			payload, GroundedAnswerParagraphAuthorizationInput.validate,
		)
	case WorkDatabaseSchemaRelationInventory:
		return decodeAndValidatePortablePayload[DatabaseSchemaRelationInventoryInput](
			payload, DatabaseSchemaRelationInventoryInput.validate,
		)
	case WorkDatabaseSchemaRelationNecessity:
		return decodeAndValidatePortablePayload[DatabaseSchemaRelationNecessityInput](
			payload, DatabaseSchemaRelationNecessityInput.validate,
		)
	case WorkDatabaseSchemaRelationResolution:
		return decodeAndValidatePortablePayload[DatabaseSchemaRelationResolutionInput](
			payload, DatabaseSchemaRelationResolutionInput.validate,
		)
	case WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape:
		return decodeAndValidatePortablePayload[DatabaseQueryIntentLeafState](
			payload, DatabaseQueryIntentLeafState.validate,
		)
	case WorkDatabaseQueryPurposeInventory:
		return decodeAndValidatePortablePayload[DatabaseQueryPurposeAuthority](
			payload, DatabaseQueryPurposeAuthority.validate,
		)
	case WorkDatabaseQueryPurposeNecessity:
		return decodeAndValidatePortablePayload[DatabaseQueryPurposeNecessityInput](
			payload, DatabaseQueryPurposeNecessityInput.validate,
		)
	case WorkDatabaseQueryPurposeRelation:
		return decodeAndValidatePortablePayload[DatabaseQueryPurposeRelationInput](
			payload, DatabaseQueryPurposeRelationInput.validate,
		)
	case WorkDatabaseQueryProjectionAggregate:
		return decodeAndValidatePortablePayload[DatabaseQueryProjectionLeafInput](
			payload, DatabaseQueryProjectionLeafInput.validate,
		)
	case WorkDatabaseQueryProjectionField:
		return decodeAndValidatePortablePayload[DatabaseQueryProjectionLeafInput](
			payload, DatabaseQueryProjectionLeafInput.validateForField,
		)
	case WorkDatabaseQueryProjectionTimeBucket:
		return decodeAndValidatePortablePayload[DatabaseQueryProjectionLeafInput](
			payload, DatabaseQueryProjectionLeafInput.validateForTimeBucket,
		)
	case WorkDatabaseQueryFilterField:
		return decodeAndValidatePortablePayload[DatabaseQueryFilterLeafInput](
			payload, DatabaseQueryFilterLeafInput.validate,
		)
	case WorkDatabaseQueryFilterOperator:
		return decodeAndValidatePortablePayload[DatabaseQueryFilterLeafInput](
			payload, DatabaseQueryFilterLeafInput.validateField,
		)
	case WorkDatabaseQueryFilterValue:
		return decodeAndValidatePortablePayload[DatabaseQueryFilterLeafInput](
			payload, DatabaseQueryFilterLeafInput.validateOperator,
		)
	case WorkDatabaseQueryWindowField:
		return decodeAndValidatePortablePayload[DatabaseQueryWindowLeafInput](
			payload, DatabaseQueryWindowLeafInput.validate,
		)
	case WorkDatabaseQueryWindowUnit:
		return decodeAndValidatePortablePayload[DatabaseQueryWindowLeafInput](
			payload, DatabaseQueryWindowLeafInput.validateField,
		)
	case WorkDatabaseQueryWindowAmount:
		return decodeAndValidatePortablePayload[DatabaseQueryWindowLeafInput](
			payload, DatabaseQueryWindowLeafInput.validateUnit,
		)
	case WorkDatabaseQueryExistenceRelation:
		return decodeAndValidatePortablePayload[DatabaseQueryExistenceLeafInput](
			payload, DatabaseQueryExistenceLeafInput.validate,
		)
	case WorkDatabaseQueryExistenceNegated:
		return decodeAndValidatePortablePayload[DatabaseQueryExistenceLeafInput](
			payload, DatabaseQueryExistenceLeafInput.validateRelation,
		)
	case WorkDatabaseQueryHavingAggregate:
		return decodeAndValidatePortablePayload[DatabaseQueryHavingLeafInput](
			payload, DatabaseQueryHavingLeafInput.validate,
		)
	case WorkDatabaseQueryHavingField:
		return decodeAndValidatePortablePayload[DatabaseQueryHavingLeafInput](
			payload, DatabaseQueryHavingLeafInput.validateAggregate,
		)
	case WorkDatabaseQueryHavingOperator:
		return decodeAndValidatePortablePayload[DatabaseQueryHavingLeafInput](
			payload, DatabaseQueryHavingLeafInput.validateField,
		)
	case WorkDatabaseQueryHavingValue:
		return decodeAndValidatePortablePayload[DatabaseQueryHavingLeafInput](
			payload, DatabaseQueryHavingLeafInput.validateOperator,
		)
	case WorkDatabaseQueryOrderProjection:
		return decodeAndValidatePortablePayload[DatabaseQueryOrderLeafInput](
			payload, DatabaseQueryOrderLeafInput.validate,
		)
	case WorkDatabaseQueryOrderDirection:
		return decodeAndValidatePortablePayload[DatabaseQueryOrderLeafInput](
			payload, DatabaseQueryOrderLeafInput.validateProjection,
		)
	case WorkDatabaseJoinPathSelection:
		return decodeAndValidatePortablePayload[DatabaseJoinPathSelectionInput](payload, DatabaseJoinPathSelectionInput.validate)
	case WorkWebRelevanceRelation:
		return decodeAndValidatePortablePayload[WebRelevanceRelationInput](
			payload, WebRelevanceRelationInput.validate,
		)
	case WorkArtifactHandling:
		return decodeAndValidatePortablePayload[ArtifactHandlingInput](payload, ArtifactHandlingInput.validate)
	case WorkCapabilityRelation:
		return decodeAndValidatePortablePayload[CapabilityRelationInput](payload, CapabilityRelationInput.validate)
	case WorkRuntimeCapabilityNecessity:
		return decodeAndValidatePortablePayload[RuntimeCapabilityNecessityInput](
			payload, RuntimeCapabilityNecessityInput.validate,
		)
	case WorkTypeScriptRepairGuidance:
		return decodeAndValidatePortablePayload[FragmentRepairGuidanceInput](
			payload, FragmentRepairGuidanceInput.validate,
		)
	case WorkFragmentGeneration:
		return decodeAndValidatePortablePayload[FragmentGenerationInput](payload, FragmentGenerationInput.validate)
	case WorkFragmentGenerationReplacement:
		return decodeAndValidatePortablePayload[FragmentGenerationReplacementInput](
			payload, FragmentGenerationReplacementInput.validate,
		)
	case WorkFragmentModification:
		return decodeAndValidatePortablePayload[FragmentModificationInput](payload, FragmentModificationInput.validate)
	case WorkFragmentCorrection:
		return decodeAndValidatePortablePayload[FragmentCorrectionInput](payload, FragmentCorrectionInput.validate)
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
