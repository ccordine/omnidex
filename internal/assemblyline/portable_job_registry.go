package assemblyline

const (
	WorkApplicationProjectStackConstraint        WorkKind = "application_project_stack_constraint"
	WorkApplicationTargetTree                    WorkKind = "application_target_tree"
	WorkContextMinification                      WorkKind = "context_minification"
	WorkConversationObjectiveKind                WorkKind = "conversation_objective_kind"
	WorkConversationResponse                     WorkKind = "conversation_response"
	WorkRoleplayOngoingAction                    WorkKind = "roleplay_ongoing_action"
	WorkDatabaseJoinPathSelection                WorkKind = "database_join_path_selection"
	WorkApplicationClassify                      WorkKind = "application_classification"
	WorkArtifactHandling                         WorkKind = "artifact_handling"
	WorkCapabilityRelation                       WorkKind = "capability_relation"
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
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementInventory,
		WorkRepositoryRequirementCandidateAuthorization,
		WorkRepositoryRequirementCandidateRelation,
		WorkRepositoryEvidenceRelevanceRelation,
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
		WorkArtifactHandling,
		WorkCapabilityRelation, WorkRuntimeCapabilityNecessity,
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
		WorkApplicationClassify,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementInventory,
		WorkRepositoryRequirementCandidateAuthorization,
		WorkRepositoryRequirementCandidateRelation,
		WorkRepositoryEvidenceRelevanceRelation,
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
		WorkArtifactHandling,
		WorkCapabilityRelation, WorkRuntimeCapabilityNecessity,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration, WorkFragmentGenerationReplacement,
		WorkFragmentModification, WorkFragmentCorrection,
	}
}
