package assemblyline

const (
	WorkApplicationProjectStackConstraint        WorkKind = "application_project_stack_constraint"
	WorkApplicationServiceContinuedAvailability  WorkKind = "application_service_continued_availability"
	WorkApplicationServicePersistenceDestination WorkKind = "application_service_persistence_destination"
	WorkApplicationServiceStateLifetime          WorkKind = "application_service_state_lifetime"
	WorkApplicationServiceEndpointRequirement    WorkKind = "application_service_endpoint_requirement"
	WorkApplicationServiceEndpointExposure       WorkKind = "application_service_endpoint_exposure"
	WorkApplicationServiceEndpointMethod         WorkKind = "application_service_endpoint_method"
	WorkApplicationServiceEndpointRouteTemplate  WorkKind = "application_service_endpoint_route_template"
	WorkApplicationServiceEndpointRequestMedia   WorkKind = "application_service_endpoint_request_media"
	WorkApplicationServiceEndpointResponseMedia  WorkKind = "application_service_endpoint_response_media"
	WorkApplicationServiceEndpointSuccessStatus  WorkKind = "application_service_endpoint_success_status"
	WorkApplicationTargetTree                    WorkKind = "application_target_tree"
	WorkContextMinification                      WorkKind = "context_minification"
	WorkConversationObjectiveKind                WorkKind = "conversation_objective_kind"
	WorkConversationResponse                     WorkKind = "conversation_response"
	WorkRoleplayOngoingAction                    WorkKind = "roleplay_ongoing_action"
	WorkDatabaseJoinPathSelection                WorkKind = "database_join_path_selection"
	WorkApplicationClassify                      WorkKind = "application_classification"
	WorkArtifactHandling                         WorkKind = "artifact_handling"
	WorkRepositoryArtifactAbsence                WorkKind = "repository_artifact_absence"
	WorkPlainTextArtifactCreation                WorkKind = "plain_text_artifact_creation"
	WorkDeclarationArtifactBoundary              WorkKind = "declaration_artifact_boundary"
	WorkArtifactCandidateSelection               WorkKind = "artifact_candidate_selection"
	WorkCapabilityRelation                       WorkKind = "capability_relation"
	WorkSkillSelection                           WorkKind = "skill_selection"
	WorkRuntimeCapabilityNecessity               WorkKind = "runtime_capability_necessity"
	WorkTypeScriptRepairGuidance                 WorkKind = "typescript_repair_guidance"
	WorkFragmentGeneration                       WorkKind = "fragment_generation"
	WorkFragmentGenerationReplacement            WorkKind = "fragment_generation_replacement"
	WorkFragmentModification                     WorkKind = "fragment_modification"
	WorkFragmentCorrection                       WorkKind = "fragment_correction"
)

func validWorkKind(kind WorkKind) bool {
	switch kind {
	case WorkApplicationContextQuestionInventory,
		WorkApplicationContextQuestionNecessity,
		WorkApplicationContextQuestionRelation,
		WorkApplicationProductContext,
		WorkApplicationRequirementInventory,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationGrounding,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidatePartition,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldPurposeInventory,
		WorkApplicationStateFieldKind,
		WorkApplicationRecordFieldPurposeInventory,
		WorkApplicationRecordFieldKind,
		WorkApplicationServiceStatePurposeNecessity,
		WorkApplicationServiceStatePurposeRelation,
		WorkApplicationServiceEndpointRequirement,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementInventory,
		WorkRepositoryRequirementCandidateAuthorization,
		WorkRepositoryRequirementCandidateRelation,
		WorkRepositoryEvidenceRelevanceRelation,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceRelation, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactInventory,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkRoleplayOngoingAction,
		WorkGroundedAnswerParagraphInventory,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationInventory, WorkDatabaseSchemaRelationNecessity,
		WorkDatabaseSchemaRelationResolution,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposeInventory, WorkDatabaseQueryPurposeNecessity,
		WorkDatabaseQueryPurposeRelation,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterField, WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphInventory,
		WorkWebSynthesisEvidenceRelation, WorkWebSynthesisParagraphAuthorization,
		WorkArtifactHandling, WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection, WorkRuntimeCapabilityNecessity,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration, WorkFragmentGenerationReplacement,
		WorkFragmentModification, WorkFragmentCorrection:
		return true
	default:
		return false
	}
}

// AllWorkKinds returns the closed PortableJob kind registry. Callers may use
// it to prove exhaustive station mappings without inventing string routing.
func AllWorkKinds() []WorkKind {
	return []WorkKind{
		WorkApplicationContextQuestionInventory,
		WorkApplicationContextQuestionNecessity,
		WorkApplicationContextQuestionRelation,
		WorkApplicationProductContext,
		WorkApplicationRequirementInventory,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationGrounding,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidatePartition,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldPurposeInventory,
		WorkApplicationStateFieldKind,
		WorkApplicationRecordFieldPurposeInventory,
		WorkApplicationRecordFieldKind,
		WorkApplicationServiceStatePurposeNecessity,
		WorkApplicationServiceStatePurposeRelation,
		WorkApplicationServiceEndpointRequirement,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementInventory,
		WorkRepositoryRequirementCandidateAuthorization,
		WorkRepositoryRequirementCandidateRelation,
		WorkRepositoryEvidenceRelevanceRelation,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceRelation, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactInventory,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkRoleplayOngoingAction,
		WorkGroundedAnswerParagraphInventory,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationInventory, WorkDatabaseSchemaRelationNecessity,
		WorkDatabaseSchemaRelationResolution,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposeInventory, WorkDatabaseQueryPurposeNecessity,
		WorkDatabaseQueryPurposeRelation,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterField, WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphInventory,
		WorkWebSynthesisEvidenceRelation, WorkWebSynthesisParagraphAuthorization,
		WorkArtifactHandling, WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection, WorkRuntimeCapabilityNecessity,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration, WorkFragmentGenerationReplacement,
		WorkFragmentModification, WorkFragmentCorrection,
	}
}
