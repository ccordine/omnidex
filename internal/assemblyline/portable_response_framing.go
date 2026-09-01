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
		WorkApplicationRequirementInventory,
		WorkDatabaseQueryPurposeInventory,
		WorkApplicationRequirementCandidatePartition,
		WorkContextMinification,
		WorkConversationResponse,
		WorkRoleplayGroundedResponseParagraphInventory,
		WorkRoleplayCanonFactInventory,
		WorkGroundedAnswerParagraphInventory,
		WorkFragmentGeneration:
		return PortableResponseFramingNaturalMultiline, nil
	case WorkApplicationRequirementCandidateCardinality,
		WorkApplicationRequirementCandidateKind,
		WorkApplicationRequirementCandidateAuthorization,
		WorkApplicationRequirementCandidateOutcomeRelation,
		WorkApplicationRequirementCandidateResultRelation,
		WorkApplicationProjectStackConstraint,
		WorkApplicationClassify,
		WorkContextRelevanceRelation,
		WorkConversationObjectiveKind,
		WorkRoleplayGroundedResponseEvidenceRelation,
		WorkRoleplayGroundedResponseParagraphAuthorization,
		WorkRoleplayCanonFactPresence,
		WorkRoleplayCanonFactCandidateAuthorization,
		WorkRoleplayCanonFactCandidateRelation,
		WorkRoleplayOngoingActionRelation,
		WorkRoleplayOngoingActionValue,
		WorkGroundedAnswerParagraphEvidenceRelation,
		WorkGroundedAnswerParagraphAuthorization,
		WorkDatabaseSchemaRelationChoice,
		WorkDatabaseQueryFromRelation,
		WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposePresence,
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
		WorkArtifactHandling,
		WorkCapabilityRelation:
		return PortableResponseFramingSingleLine, nil
	default:
		return "", fmt.Errorf("portable work kind %q has no registered response framing", kind)
	}
}

// PortableResponseFramingForJob returns only a provider-actionable framing
// value for one validated job.
func PortableResponseFramingForJob(job PortableJob) (PortableResponseFraming, error) {
	return PortableResponseFramingForWorkKind(job.Kind)
}
