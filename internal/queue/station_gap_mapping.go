package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func stationForPortableJob(job assemblyline.PortableJob) (station.ID, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	return stationForPortableWorkKind(job.Kind)
}

// StationForPortableJob returns the sole registered station that owns a
// validated PortableJob. The worker uses this same mapping when it opens the
// durable gap, so dispatch cannot maintain a second work-kind router.
func StationForPortableJob(job assemblyline.PortableJob) (station.ID, error) {
	return stationForPortableJob(job)
}

func stationForPortableWorkKind(kind assemblyline.WorkKind) (station.ID, error) {
	switch kind {
	case assemblyline.WorkApplicationContextQuestionInventory,
		assemblyline.WorkApplicationContextQuestionNecessity,
		assemblyline.WorkApplicationContextQuestionRelation,
		assemblyline.WorkApplicationProductContext,
		assemblyline.WorkApplicationRequirementInventory,
		assemblyline.WorkApplicationRequirementCandidateCardinality,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateOutcomeRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelation,
		assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding,
		assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection,
		assemblyline.WorkApplicationRequirementCandidatePartition,
		assemblyline.WorkRepositoryRequirementInventory,
		assemblyline.WorkRepositoryRequirementCandidateAuthorization,
		assemblyline.WorkRepositoryRequirementCandidateRelation:
		return station.CodingRequirements, nil
	case assemblyline.WorkApplicationProjectStackConstraint:
		return station.CodingProjectStackConstraint, nil
	case assemblyline.WorkApplicationServiceContinuedAvailability:
		return station.CodingServiceContinuedAvailability, nil
	case assemblyline.WorkApplicationServicePersistenceDestination:
		return station.CodingServicePersistenceDestination, nil
	case assemblyline.WorkApplicationServiceStateLifetime:
		return station.CodingServiceStateLifetime, nil
	case assemblyline.WorkApplicationStateFieldPurposeInventory:
		return station.CodingApplicationStateFieldPurposeInventory, nil
	case assemblyline.WorkApplicationStateFieldKind:
		return station.CodingApplicationStateFieldKind, nil
	case assemblyline.WorkApplicationRecordFieldPurposeInventory:
		return station.CodingApplicationRecordFieldPurposeInventory, nil
	case assemblyline.WorkApplicationRecordFieldKind:
		return station.CodingApplicationRecordFieldKind, nil
	case assemblyline.WorkApplicationServiceStatePurposeNecessity:
		return station.CodingApplicationServiceStatePurposeNecessity, nil
	case assemblyline.WorkApplicationServiceStatePurposeRelation:
		return station.CodingApplicationServiceStatePurposeRelation, nil
	case assemblyline.WorkApplicationServiceEndpointRequirement:
		return station.CodingServiceEndpointRequirement, nil
	case assemblyline.WorkApplicationServiceEndpointExposure:
		return station.CodingServiceEndpointExposure, nil
	case assemblyline.WorkApplicationServiceEndpointMethod:
		return station.CodingServiceEndpointMethod, nil
	case assemblyline.WorkApplicationServiceEndpointRouteTemplate:
		return station.CodingServiceEndpointRouteTemplate, nil
	case assemblyline.WorkApplicationServiceEndpointRequestMedia:
		return station.CodingServiceEndpointRequestMedia, nil
	case assemblyline.WorkApplicationServiceEndpointResponseMedia:
		return station.CodingServiceEndpointResponseMedia, nil
	case assemblyline.WorkApplicationServiceEndpointSuccessStatus:
		return station.CodingServiceEndpointSuccessStatus, nil
	case assemblyline.WorkApplicationClassify:
		return station.CodingSurface, nil
	case assemblyline.WorkApplicationTargetTree:
		return station.CodingTargetTree, nil
	case assemblyline.WorkRepositoryChangeOwner:
		return station.CodingRepositoryChange, nil
	case assemblyline.WorkContextRelevanceRelation:
		return station.ContextRelevance, nil
	case assemblyline.WorkContextMinification:
		return station.ContextMinification, nil
	case assemblyline.WorkConversationObjectiveKind:
		return station.ConversationObjectiveKind, nil
	case assemblyline.WorkConversationResponse,
		assemblyline.WorkRoleplayGroundedResponseParagraphInventory,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation,
		assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization:
		return station.ConversationResponse, nil
	case assemblyline.WorkRoleplayCanonFactInventory,
		assemblyline.WorkRoleplayCanonFactCandidateAuthorization,
		assemblyline.WorkRoleplayCanonFactCandidateRelation:
		return station.RoleplayCanonExtraction, nil
	case assemblyline.WorkRoleplayOngoingAction:
		return station.RoleplayOngoingAction, nil
	case assemblyline.WorkGroundedAnswerParagraphInventory,
		assemblyline.WorkGroundedAnswerParagraphEvidenceRelation,
		assemblyline.WorkGroundedAnswerParagraphAuthorization:
		return station.GroundedAnswer, nil
	case assemblyline.WorkDatabaseSchemaRelationInventory,
		assemblyline.WorkDatabaseSchemaRelationNecessity,
		assemblyline.WorkDatabaseSchemaRelationResolution:
		return station.DatabaseSchemaSelection, nil
	case assemblyline.WorkDatabaseQueryFromRelation,
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryPurposeInventory,
		assemblyline.WorkDatabaseQueryPurposeNecessity,
		assemblyline.WorkDatabaseQueryPurposeRelation,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryProjectionField,
		assemblyline.WorkDatabaseQueryProjectionTimeBucket,
		assemblyline.WorkDatabaseQueryFilterField,
		assemblyline.WorkDatabaseQueryFilterOperator,
		assemblyline.WorkDatabaseQueryFilterValue,
		assemblyline.WorkDatabaseQueryWindowField,
		assemblyline.WorkDatabaseQueryWindowUnit,
		assemblyline.WorkDatabaseQueryWindowAmount,
		assemblyline.WorkDatabaseQueryExistenceRelation,
		assemblyline.WorkDatabaseQueryExistenceNegated,
		assemblyline.WorkDatabaseQueryHavingAggregate,
		assemblyline.WorkDatabaseQueryHavingField,
		assemblyline.WorkDatabaseQueryHavingOperator,
		assemblyline.WorkDatabaseQueryHavingValue,
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection:
		return station.DatabaseQueryIntent, nil
	case assemblyline.WorkDatabaseJoinPathSelection:
		return station.DatabaseJoinPathSelection, nil
	case assemblyline.WorkRepositoryEvidenceRelevanceRelation:
		return station.RepositoryEvidenceRelevance, nil
	case assemblyline.WorkWebRelevanceRelation:
		return station.WebRelevance, nil
	case assemblyline.WorkWebSynthesisParagraphInventory,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisParagraphAuthorization:
		return station.WebGroundedSynthesis, nil
	case assemblyline.WorkArtifactHandling:
		return station.CodingArtifactHandling, nil
	case assemblyline.WorkRepositoryArtifactAbsence:
		return station.CodingRepositoryArtifactAbsence, nil
	case assemblyline.WorkPlainTextArtifactCreation:
		return station.CodingPlainTextArtifactCreation, nil
	case assemblyline.WorkDeclarationArtifactBoundary:
		return station.CodingDeclarationArtifactBoundary, nil
	case assemblyline.WorkArtifactCandidateSelection:
		return station.CodingArtifactCandidateSelection, nil
	case assemblyline.WorkCapabilityRelation:
		return station.CodingCapabilityRelation, nil
	case assemblyline.WorkSkillSelection:
		return station.CodingSkillSelection, nil
	case assemblyline.WorkRuntimeCapabilityNecessity:
		return station.CodingRuntimeCapabilityNecessity, nil
	case assemblyline.WorkTypeScriptRepairGuidance:
		return station.CodingFragmentRepairGuidance, nil
	case assemblyline.WorkFragmentGeneration,
		assemblyline.WorkFragmentGenerationReplacement,
		assemblyline.WorkFragmentModification:
		return station.CodingFragment, nil
	case assemblyline.WorkFragmentCorrection:
		return station.CodingFragmentCorrection, nil
	default:
		return "", fmt.Errorf("portable work kind %q is not a production semantic station", kind)
	}
}
