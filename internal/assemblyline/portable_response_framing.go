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
	case WorkApplicationProductContext,
		WorkApplicationRequirement,
		WorkApplicationTargetTree,
		WorkRepositoryRequirement,
		WorkContextMinification,
		WorkConversationResponse,
		WorkRoleplayGroundedResponseText,
		WorkRoleplayOngoingAction,
		WorkGroundedAnswerText,
		WorkDatabaseEvidenceGap,
		WorkWebSynthesisParagraph,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration,
		WorkFragmentModification,
		WorkFragmentCorrection:
		return PortableResponseFramingNaturalMultiline, nil
	case WorkApplicationContextNeedCoverage,
		WorkApplicationContextNeedQuestion,
		WorkApplicationRequirementCoverage,
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
		WorkRepositoryRequirementCoverage,
		WorkRepositoryEvidenceRelevanceLeaf,
		WorkRepositoryChangeOwner,
		WorkContextRelevanceSelection,
		WorkConversationObjectiveKind,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayCanonFactCoverage,
		WorkRoleplayCanonFact,
		WorkGroundedAnswerEvidenceRelation,
		WorkDatabaseSchemaSelectionCoverage,
		WorkDatabaseSchemaRelationSelection,
		WorkDatabaseQueryFromRelation,
		WorkDatabaseQueryShape,
		WorkDatabaseQueryProjectionCoverage,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField,
		WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterCoverage,
		WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValueCoverage,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryWindowField,
		WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceCoverage,
		WorkDatabaseQueryExistenceRelation,
		WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingCoverage,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField,
		WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderCoverage,
		WorkDatabaseQueryOrderProjection,
		WorkDatabaseQueryOrderDirection,
		WorkDatabaseJoinPathSelection,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphCoverage,
		WorkWebSynthesisEvidenceRelation,
		WorkArtifactHandling,
		WorkRepositoryArtifactAbsence,
		WorkPlainTextArtifactCreation,
		WorkDeclarationArtifactBoundary,
		WorkArtifactCandidateSelection,
		WorkCapabilityRelation,
		WorkSkillSelection:
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
