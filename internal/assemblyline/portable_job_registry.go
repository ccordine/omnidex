package assemblyline

const (
	WorkApplicationProjectStackConstraint WorkKind = "application_project_stack_constraint"
	WorkContextMinification               WorkKind = "context_minification"
	WorkConversationObjectiveKind         WorkKind = "conversation_objective_kind"
	WorkConversationResponse              WorkKind = "conversation_response"
	WorkRoleplayOngoingActionRelation     WorkKind = "roleplay_ongoing_action_relation"
	WorkRoleplayOngoingActionValue        WorkKind = "roleplay_ongoing_action_value"
	WorkDatabaseJoinPathSelection         WorkKind = "database_join_path_selection"
	WorkApplicationClassify               WorkKind = "application_classification"
	WorkArtifactHandling                  WorkKind = "artifact_handling"
	WorkCapabilityRelation                WorkKind = "capability_relation"
	WorkFragmentGeneration                WorkKind = "fragment_generation"
)

func validWorkKind(kind WorkKind) bool {
	switch kind {
	case WorkApplicationProductContext,
		WorkApplicationRequirementInventory,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidatePartition,
		WorkApplicationProjectStackConstraint,
		WorkApplicationClassify,
		WorkContextRelevanceRelation, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactPresence,
		WorkRoleplayCanonFactInventory,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkRoleplayOngoingActionRelation,
		WorkRoleplayOngoingActionValue,
		WorkGroundedAnswerParagraphInventory,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationChoice,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposePresence,
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
		WorkArtifactHandling,
		WorkCapabilityRelation,
		WorkFragmentGeneration:
		return true
	default:
		return false
	}
}

// AllWorkKinds returns the closed PortableJob kind registry. Callers may use
// it to prove exhaustive station mappings without inventing string routing.
func AllWorkKinds() []WorkKind {
	return []WorkKind{
		WorkApplicationProductContext,
		WorkApplicationRequirementInventory,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidatePartition,
		WorkApplicationProjectStackConstraint,
		WorkApplicationClassify,
		WorkContextRelevanceRelation, WorkContextMinification,
		WorkConversationObjectiveKind, WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactPresence,
		WorkRoleplayCanonFactInventory,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkRoleplayOngoingActionRelation,
		WorkRoleplayOngoingActionValue,
		WorkGroundedAnswerParagraphInventory,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationChoice,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposePresence,
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
		WorkArtifactHandling,
		WorkCapabilityRelation,
		WorkFragmentGeneration,
	}
}
