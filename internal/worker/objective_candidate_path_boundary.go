package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// validateObjectiveRawCandidatePathBoundary is the closed raw-result policy
// for stations dispatched by the objective runtime. Most leaves are semantic
// prose, registered values, or opaque IDs and must be path-free before their
// station decoder sees them. The two database scalar-value leaves are typed
// database data, not filesystem identities; their owning decoders validate
// and bind them to the selected schema field.
func validateObjectiveRawCandidatePathBoundary(
	kind assemblyline.WorkKind,
	candidate string,
	provenance assemblyline.ArtifactIdentityProvenance,
) error {
	switch kind {
	case assemblyline.WorkDatabaseQueryFilterValue,
		assemblyline.WorkDatabaseQueryHavingValue:
		return nil
	case assemblyline.WorkContextRelevanceSelection,
		assemblyline.WorkContextMinification,
		assemblyline.WorkConversationObjectiveKind,
		assemblyline.WorkConversationResponse,
		assemblyline.WorkRoleplayGroundedResponseText,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayCanonFactCoverage,
		assemblyline.WorkRoleplayCanonFact,
		assemblyline.WorkRoleplayOngoingAction,
		assemblyline.WorkGroundedAnswerText,
		assemblyline.WorkGroundedAnswerEvidenceRelation,
		assemblyline.WorkRepositoryEvidenceRelevanceLeaf,
		assemblyline.WorkDatabaseSchemaSelectionCoverage,
		assemblyline.WorkDatabaseSchemaRelationSelection,
		assemblyline.WorkDatabaseQueryFromRelation,
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryProjectionCoverage,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryProjectionField,
		assemblyline.WorkDatabaseQueryProjectionTimeBucket,
		assemblyline.WorkDatabaseQueryFilterCoverage,
		assemblyline.WorkDatabaseQueryFilterField,
		assemblyline.WorkDatabaseQueryFilterOperator,
		assemblyline.WorkDatabaseQueryFilterValueCoverage,
		assemblyline.WorkDatabaseQueryWindowCoverage,
		assemblyline.WorkDatabaseQueryWindowField,
		assemblyline.WorkDatabaseQueryWindowUnit,
		assemblyline.WorkDatabaseQueryWindowAmount,
		assemblyline.WorkDatabaseQueryExistenceCoverage,
		assemblyline.WorkDatabaseQueryExistenceRelation,
		assemblyline.WorkDatabaseQueryExistenceNegated,
		assemblyline.WorkDatabaseQueryHavingCoverage,
		assemblyline.WorkDatabaseQueryHavingAggregate,
		assemblyline.WorkDatabaseQueryHavingField,
		assemblyline.WorkDatabaseQueryHavingOperator,
		assemblyline.WorkDatabaseQueryOrderCoverage,
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection,
		assemblyline.WorkDatabaseEvidenceGap,
		assemblyline.WorkDatabaseJoinPathSelection,
		assemblyline.WorkWebRelevanceRelation,
		assemblyline.WorkWebSynthesisParagraphCoverage,
		assemblyline.WorkWebSynthesisParagraph,
		assemblyline.WorkWebSynthesisEvidenceRelation:
		return assemblyline.ValidatePathFreeModelContextWithProvenance(
			"objective semantic raw result", provenance, candidate,
		)
	default:
		return fmt.Errorf(
			"portable work kind %q has no objective raw-result path contract", kind,
		)
	}
}
