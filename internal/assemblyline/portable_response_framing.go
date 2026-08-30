package assemblyline

import "fmt"

// PortableResponseFraming is code-owned authority for provider termination.
// It describes only the byte grammar of one result, never its meaning.
type PortableResponseFraming string

const (
	PortableResponseFramingSingleLine       PortableResponseFraming = "single_line"
	PortableResponseFramingNaturalMultiline PortableResponseFraming = "natural_multiline"
)

// PortableResponseFramingForWorkKind classifies every registered work kind by
// its exact result grammar.
func PortableResponseFramingForWorkKind(
	kind WorkKind,
) (PortableResponseFraming, error) {
	switch kind {
	case WorkApplicationContextQuestionInventory,
		WorkApplicationProductContext,
		WorkApplicationRequirementInventory,
		WorkApplicationStateFieldPurposeInventory,
		WorkApplicationRecordFieldPurposeInventory,
		WorkDatabaseSchemaRelationInventory,
		WorkDatabaseQueryPurposeInventory,
		WorkApplicationRequirementCandidateResultRelationCorrection,
		WorkApplicationRequirementCandidatePartition,
		WorkApplicationTargetTree,
		WorkRepositoryRequirementInventory,
		WorkContextMinification,
		WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayCanonFactInventory,
		WorkRoleplayOngoingAction,
		WorkGroundedAnswerParagraphInventory,
		WorkWebSynthesisParagraphInventory,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration,
		WorkFragmentGenerationReplacement,
		WorkFragmentModification,
		WorkFragmentCorrection:
		return PortableResponseFramingNaturalMultiline, nil
	case WorkApplicationContextQuestionNecessity,
		WorkApplicationContextQuestionRelation,
		WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationRequirementCandidateResultRelationGrounding,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldKind,
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
		WorkRepositoryRequirementCandidateAuthorization,
		WorkRepositoryRequirementCandidateRelation,
		WorkRepositoryEvidenceRelevanceRelation,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceRelation,
		WorkConversationObjectiveKind,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationNecessity,
		WorkDatabaseSchemaRelationResolution,
		WorkDatabaseQueryFromRelation,
		WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposeNecessity,
		WorkDatabaseQueryPurposeRelation,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField,
		WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowField,
		WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceRelation,
		WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField,
		WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderProjection,
		WorkDatabaseQueryOrderDirection,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisEvidenceRelation,
		WorkWebSynthesisParagraphAuthorization,
		WorkArtifactHandling,
		WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary,
		WorkArtifactCandidateSelection,
		WorkCapabilityRelation,
		WorkSkillSelection,
		WorkRuntimeCapabilityNecessity:
		return PortableResponseFramingSingleLine, nil
	default:
		return "", fmt.Errorf("portable work kind %q has no registered response framing", kind)
	}
}

// PortableResponseFramingForJob returns only a provider-actionable framing
// value for one validated job.
func PortableResponseFramingForJob(job PortableJob) (PortableResponseFraming, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	return PortableResponseFramingForWorkKind(job.Kind)
}
