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
	case assemblyline.WorkContextRelevanceRelation,
		assemblyline.WorkContextMinification,
		assemblyline.WorkConversationObjectiveKind,
		assemblyline.WorkConversationResponse,
		assemblyline.WorkRoleplayGroundedResponseParagraphInventory,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization,
		assemblyline.WorkRoleplayCanonFactInventory,
		assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
		assemblyline.WorkRoleplayCanonFactCandidateRelation,
		assemblyline.WorkRoleplayOngoingAction,
		assemblyline.WorkGroundedAnswerParagraphInventory,
		assemblyline.WorkGroundedAnswerParagraphEvidenceRelation,
		assemblyline.WorkGroundedAnswerParagraphAuthorization,
		assemblyline.WorkDatabaseSchemaRelationInventory,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationResolution,
		assemblyline.WorkDatabaseQueryFromRelation,
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeRelation,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryProjectionField,
		assemblyline.WorkDatabaseQueryProjectionTimeBucket,
		assemblyline.WorkDatabaseQueryFilterField,
		assemblyline.WorkDatabaseQueryFilterOperator,
		assemblyline.WorkDatabaseQueryWindowField,
		assemblyline.WorkDatabaseQueryWindowUnit,
		assemblyline.WorkDatabaseQueryWindowAmount,
		assemblyline.WorkDatabaseQueryExistenceRelation,
		assemblyline.WorkDatabaseQueryExistenceNegated,
		assemblyline.WorkDatabaseQueryHavingAggregate,
		assemblyline.WorkDatabaseQueryHavingField,
		assemblyline.WorkDatabaseQueryHavingOperator,
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection,
		assemblyline.WorkDatabaseJoinPathSelection,
		assemblyline.WorkWebRelevanceRelation:
		return assemblyline.ValidatePathFreeModelContextWithProvenance(
			"objective semantic raw result", provenance, candidate,
		)
	default:
		return fmt.Errorf(
			"portable work kind %q has no objective raw-result path contract", kind,
		)
	}
}
