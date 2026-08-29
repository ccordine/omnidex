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
	WorkDatabaseEvidenceGap                      WorkKind = "database_evidence_gap"
	WorkDatabaseJoinPathSelection                WorkKind = "database_join_path_selection"
	WorkApplicationClassify                      WorkKind = "application_classification"
	WorkArtifactHandling                         WorkKind = "artifact_handling"
	WorkRepositoryArtifactAbsence                WorkKind = "repository_artifact_absence"
	WorkPlainTextArtifactCreation                WorkKind = "plain_text_artifact_creation"
	WorkDeclarationArtifactBoundary              WorkKind = "declaration_artifact_boundary"
	WorkArtifactCandidateSelection               WorkKind = "artifact_candidate_selection"
	WorkCapabilityRelation                       WorkKind = "capability_relation"
	WorkSkillSelection                           WorkKind = "skill_selection"
	WorkRuntimeCapabilitySelection               WorkKind = "runtime_capability_selection"
	WorkTypeScriptRepairGuidance                 WorkKind = "typescript_repair_guidance"
	WorkFragmentGeneration                       WorkKind = "fragment_generation"
	WorkFragmentGenerationReplacement            WorkKind = "fragment_generation_replacement"
	WorkFragmentModification                     WorkKind = "fragment_modification"
	WorkFragmentCorrection                       WorkKind = "fragment_correction"
)

func validWorkKind(kind WorkKind) bool {
	switch kind {
	case WorkApplicationContextNeedCoverage,
		WorkApplicationContextNeedQuestion,
		WorkApplicationProductContext,
		WorkApplicationRequirementCoverage,
		WorkApplicationRequirement,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidateSplit,
		WorkApplicationRequirementCandidateSplitCorrection,
		WorkApplicationRequirementCandidateDuplicateReplacement,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldCoverage,
		WorkApplicationStateFieldPurpose,
		WorkApplicationStateFieldKind,
		WorkApplicationRecordFieldCoverage,
		WorkApplicationRecordFieldPurpose,
		WorkApplicationRecordFieldKind,
		WorkApplicationServiceEndpointRequirement,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementCoverage,
		WorkRepositoryRequirement,
		WorkRepositoryEvidenceRelevanceLeaf,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceSelection, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseText, WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayCanonFactCoverage, WorkRoleplayCanonFact, WorkRoleplayOngoingAction,
		WorkGroundedAnswerText, WorkGroundedAnswerEvidenceRelation,
		WorkDatabaseSchemaSelectionCoverage, WorkDatabaseSchemaRelationSelection,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryProjectionCoverage, WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterCoverage, WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator, WorkDatabaseQueryFilterValueCoverage,
		WorkDatabaseQueryFilterValue, WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount, WorkDatabaseQueryExistenceCoverage,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingCoverage, WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue, WorkDatabaseQueryOrderCoverage,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection,
		WorkDatabaseEvidenceGap,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphCoverage, WorkWebSynthesisParagraph,
		WorkWebSynthesisEvidenceRelation,
		WorkArtifactHandling, WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection, WorkRuntimeCapabilitySelection,
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
		WorkApplicationContextNeedCoverage,
		WorkApplicationContextNeedQuestion,
		WorkApplicationProductContext,
		WorkApplicationRequirementCoverage,
		WorkApplicationRequirement,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidateSplit,
		WorkApplicationRequirementCandidateSplitCorrection,
		WorkApplicationRequirementCandidateDuplicateReplacement,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldCoverage,
		WorkApplicationStateFieldPurpose,
		WorkApplicationStateFieldKind,
		WorkApplicationRecordFieldCoverage,
		WorkApplicationRecordFieldPurpose,
		WorkApplicationRecordFieldKind,
		WorkApplicationServiceEndpointRequirement,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementCoverage,
		WorkRepositoryRequirement,
		WorkRepositoryEvidenceRelevanceLeaf,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceSelection, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseText, WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayCanonFactCoverage, WorkRoleplayCanonFact, WorkRoleplayOngoingAction,
		WorkGroundedAnswerText, WorkGroundedAnswerEvidenceRelation,
		WorkDatabaseSchemaSelectionCoverage, WorkDatabaseSchemaRelationSelection,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryProjectionCoverage, WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterCoverage, WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator, WorkDatabaseQueryFilterValueCoverage,
		WorkDatabaseQueryFilterValue, WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount, WorkDatabaseQueryExistenceCoverage,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingCoverage, WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue, WorkDatabaseQueryOrderCoverage,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection,
		WorkDatabaseEvidenceGap,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphCoverage, WorkWebSynthesisParagraph,
		WorkWebSynthesisEvidenceRelation,
		WorkArtifactHandling, WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary, WorkArtifactCandidateSelection,
		WorkCapabilityRelation, WorkSkillSelection, WorkRuntimeCapabilitySelection,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration, WorkFragmentGenerationReplacement,
		WorkFragmentModification, WorkFragmentCorrection,
	}
}
