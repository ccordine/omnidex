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
	case WorkApplicationRequirementCoverage:
		return decodeAndValidatePortablePayload[ApplicationRequirementCoverageInput](
			payload, ApplicationRequirementCoverageInput.validate,
		)
	case WorkApplicationRequirement:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateInput](
			payload, ApplicationRequirementCandidateInput.validate,
		)
	case WorkApplicationRequirementCandidateCardinality:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateCardinalityInput](
			payload, ApplicationRequirementCandidateCardinalityInput.validate,
		)
	case WorkApplicationRequirementCandidateKind:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateKindInput](
			payload, ApplicationRequirementCandidateKindInput.validate,
		)
	case WorkApplicationRequirementCandidateSplit:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateSplitInput](
			payload, ApplicationRequirementCandidateSplitInput.validate,
		)
	case WorkApplicationRequirementCandidateSplitCorrection:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateSplitCorrectionInput](
			payload, ApplicationRequirementCandidateSplitCorrectionInput.validate,
		)
	case WorkApplicationRequirementCandidateDuplicateReplacement:
		return decodeAndValidatePortablePayload[ApplicationRequirementCandidateDuplicateReplacementInput](
			payload, ApplicationRequirementCandidateDuplicateReplacementInput.validate,
		)
	case WorkApplicationContextNeedCoverage, WorkApplicationContextNeedQuestion:
		return decodeAndValidatePortablePayload[ApplicationContextNeedLeafInput](
			payload, ApplicationContextNeedLeafInput.validate,
		)
	case WorkApplicationProjectStackConstraint:
		return decodeAndValidatePortablePayload[ApplicationProjectStackConstraintInput](
			payload, ApplicationProjectStackConstraintInput.validate,
		)
	case WorkApplicationServiceContinuedAvailability:
		return decodeAndValidatePortablePayload[ApplicationServiceContinuedAvailabilityInput](
			payload, ApplicationServiceContinuedAvailabilityInput.validate,
		)
	case WorkApplicationServicePersistenceDestination:
		return decodeAndValidatePortablePayload[ApplicationServicePersistenceDestinationInput](
			payload, ApplicationServicePersistenceDestinationInput.validate,
		)
	case WorkApplicationServiceStateLifetime:
		return decodeAndValidatePortablePayload[ApplicationServiceStateLifetimeInput](
			payload, ApplicationServiceStateLifetimeInput.validate,
		)
	case WorkApplicationStateFieldCoverage, WorkApplicationStateFieldPurpose:
		return decodeAndValidatePortablePayload[ApplicationStateFieldLeafInput](
			payload, ApplicationStateFieldLeafInput.validate,
		)
	case WorkApplicationStateFieldKind:
		return decodeAndValidatePortablePayload[ApplicationStateFieldKindInput](
			payload, ApplicationStateFieldKindInput.validate,
		)
	case WorkApplicationRecordFieldCoverage, WorkApplicationRecordFieldPurpose:
		return decodeAndValidatePortablePayload[ApplicationRecordFieldLeafInput](
			payload, ApplicationRecordFieldLeafInput.validate,
		)
	case WorkApplicationRecordFieldKind:
		return decodeAndValidatePortablePayload[ApplicationRecordFieldKindInput](
			payload, ApplicationRecordFieldKindInput.validate,
		)
	case WorkApplicationServiceEndpointRequirement:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointRequirementInput](
			payload, ApplicationServiceEndpointRequirementInput.validate,
		)
	case WorkApplicationServiceEndpointExposure:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointExposureInput](
			payload, ApplicationServiceEndpointExposureInput.validate,
		)
	case WorkApplicationServiceEndpointMethod:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointMethodInput](
			payload, ApplicationServiceEndpointMethodInput.validate,
		)
	case WorkApplicationServiceEndpointRouteTemplate:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointRouteTemplateInput](
			payload, ApplicationServiceEndpointRouteTemplateInput.validate,
		)
	case WorkApplicationServiceEndpointRequestMedia:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointRequestMediaInput](
			payload, ApplicationServiceEndpointRequestMediaInput.validate,
		)
	case WorkApplicationServiceEndpointResponseMedia:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointResponseMediaInput](
			payload, ApplicationServiceEndpointResponseMediaInput.validate,
		)
	case WorkApplicationServiceEndpointSuccessStatus:
		return decodeAndValidatePortablePayload[ApplicationServiceEndpointSuccessStatusInput](
			payload, ApplicationServiceEndpointSuccessStatusInput.validate,
		)
	case WorkApplicationClassify:
		return decodeAndValidatePortablePayload[ApplicationClassificationInput](payload, ApplicationClassificationInput.validate)
	case WorkApplicationTargetTree:
		return decodeAndValidatePortablePayload[TargetTreeInput](payload, TargetTreeInput.Validate)
	case WorkRepositoryRequirementCoverage, WorkRepositoryRequirement:
		return decodeAndValidatePortablePayload[RepositoryRequirementLeafInput](
			payload, RepositoryRequirementLeafInput.validate,
		)
	case WorkRepositoryChangeOwner:
		return decodeAndValidatePortablePayload[RepositoryChangeOwnerInput](
			payload, RepositoryChangeOwnerInput.validate,
		)
	case WorkRepositoryEvidenceRelevanceLeaf:
		return decodeAndValidatePortablePayload[RepositoryEvidenceRelevanceLeafInput](
			payload, RepositoryEvidenceRelevanceLeafInput.validate,
		)
	case WorkContextRelevanceSelection:
		return decodeAndValidatePortablePayload[ContextRelevanceSelectionInput](
			payload, ContextRelevanceSelectionInput.validate,
		)
	case WorkContextMinification:
		return decodeAndValidatePortablePayload[ContextMinificationInput](payload, ContextMinificationInput.validate)
	case WorkConversationObjectiveKind:
		return decodeAndValidatePortablePayload[ConversationObjectiveKindInput](payload, ConversationObjectiveKindInput.validate)
	case WorkConversationResponse:
		return decodeAndValidatePortablePayload[ConversationResponseInput](payload, ConversationResponseInput.validate)
	case WorkRoleplayGroundedResponseText:
		return decodeAndValidatePortablePayload[RoleplayGroundedResponseInput](
			payload, RoleplayGroundedResponseInput.validate,
		)
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return decodeAndValidatePortablePayload[RoleplayGroundedEvidenceRelationInput](
			payload, RoleplayGroundedEvidenceRelationInput.validate,
		)
	case WorkRoleplayCanonFactCoverage, WorkRoleplayCanonFact:
		return decodeAndValidatePortablePayload[RoleplayCanonFactLeafInput](
			payload, RoleplayCanonFactLeafInput.validate,
		)
	case WorkRoleplayOngoingAction:
		return decodeAndValidatePortablePayload[RoleplayOngoingActionInput](
			payload, RoleplayOngoingActionInput.validate,
		)
	case WorkGroundedAnswerText:
		return decodeAndValidatePortablePayload[GroundedAnswerTextInput](
			payload, GroundedAnswerTextInput.validate,
		)
	case WorkGroundedAnswerEvidenceRelation:
		return decodeAndValidatePortablePayload[GroundedAnswerEvidenceRelationInput](
			payload, GroundedAnswerEvidenceRelationInput.validate,
		)
	case WorkDatabaseSchemaSelectionCoverage, WorkDatabaseSchemaRelationSelection:
		return decodeAndValidatePortablePayload[DatabaseSchemaSelectionLeafInput](
			payload, DatabaseSchemaSelectionLeafInput.validate,
		)
	case WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape:
		return decodeAndValidatePortablePayload[DatabaseQueryIntentLeafState](
			payload, DatabaseQueryIntentLeafState.validate,
		)
	case WorkDatabaseQueryProjectionCoverage, WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryExistenceCoverage, WorkDatabaseQueryHavingCoverage,
		WorkDatabaseQueryOrderCoverage:
		return decodeAndValidatePortablePayload[DatabaseQueryIntentLeafState](
			payload, DatabaseQueryIntentLeafState.validateReady,
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
	case WorkDatabaseQueryFilterCoverage, WorkDatabaseQueryFilterField:
		return decodeAndValidatePortablePayload[DatabaseQueryFilterLeafInput](
			payload, DatabaseQueryFilterLeafInput.validate,
		)
	case WorkDatabaseQueryFilterOperator:
		return decodeAndValidatePortablePayload[DatabaseQueryFilterLeafInput](
			payload, DatabaseQueryFilterLeafInput.validateField,
		)
	case WorkDatabaseQueryFilterValueCoverage, WorkDatabaseQueryFilterValue:
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
	case WorkDatabaseEvidenceGap:
		return decodeAndValidatePortablePayload[DatabaseEvidenceGapInput](payload, DatabaseEvidenceGapInput.validate)
	case WorkDatabaseJoinPathSelection:
		return decodeAndValidatePortablePayload[DatabaseJoinPathSelectionInput](payload, DatabaseJoinPathSelectionInput.validate)
	case WorkWebRelevanceRelation:
		return decodeAndValidatePortablePayload[WebRelevanceRelationInput](
			payload, WebRelevanceRelationInput.validate,
		)
	case WorkWebSynthesisParagraphCoverage, WorkWebSynthesisParagraph:
		return decodeAndValidatePortablePayload[WebSynthesisParagraphLeafInput](
			payload, WebSynthesisParagraphLeafInput.validate,
		)
	case WorkWebSynthesisEvidenceRelation:
		return decodeAndValidatePortablePayload[WebSynthesisEvidenceRelationInput](
			payload, WebSynthesisEvidenceRelationInput.validate,
		)
	case WorkArtifactHandling:
		return decodeAndValidatePortablePayload[ArtifactHandlingInput](payload, ArtifactHandlingInput.validate)
	case WorkRepositoryArtifactAbsence:
		return decodeAndValidatePortablePayload[RepositoryArtifactAbsenceInput](
			payload, RepositoryArtifactAbsenceInput.validate,
		)
	case WorkPlainTextArtifactCreation:
		return decodeAndValidatePortablePayload[PlainTextArtifactCreationInput](
			payload, PlainTextArtifactCreationInput.validate,
		)
	case WorkDeclarationArtifactBoundary:
		return decodeAndValidatePortablePayload[DeclarationArtifactBoundaryInput](payload, DeclarationArtifactBoundaryInput.validate)
	case WorkArtifactCandidateSelection:
		return decodeAndValidatePortablePayload[ArtifactCandidateSelectionInput](payload, ArtifactCandidateSelectionInput.validate)
	case WorkCapabilityRelation:
		return decodeAndValidatePortablePayload[CapabilityRelationInput](payload, CapabilityRelationInput.validate)
	case WorkSkillSelection:
		return decodeAndValidatePortablePayload[SkillSelectionInput](payload, SkillSelectionInput.validate)
	case WorkRuntimeCapabilitySelection:
		return decodeAndValidatePortablePayload[RuntimeCapabilitySelectionInput](
			payload, RuntimeCapabilitySelectionInput.validate,
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
