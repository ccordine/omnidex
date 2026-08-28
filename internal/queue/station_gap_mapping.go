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
	if job.Kind == assemblyline.WorkResponseCorrection {
		var input assemblyline.ResponseCorrectionInput
		if err := decodePortableGapPayload(job.Payload, &input); err != nil {
			return "", err
		}
		return stationForPortableJob(input.Original)
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
	case assemblyline.WorkApplicationContextNeedCoverage,
		assemblyline.WorkApplicationContextNeedQuestion,
		assemblyline.WorkApplicationProductContext,
		assemblyline.WorkApplicationRequirementCoverage,
		assemblyline.WorkApplicationRequirement,
		assemblyline.WorkRepositoryRequirementCoverage,
		assemblyline.WorkRepositoryRequirement:
		return station.CodingRequirements, nil
	case assemblyline.WorkApplicationProjectStackConstraint:
		return station.CodingProjectStackConstraint, nil
	case assemblyline.WorkApplicationServiceContinuedAvailability:
		return station.CodingServiceContinuedAvailability, nil
	case assemblyline.WorkApplicationServicePersistenceDestination:
		return station.CodingServicePersistenceDestination, nil
	case assemblyline.WorkApplicationServiceStateLifetime:
		return station.CodingServiceStateLifetime, nil
	case assemblyline.WorkApplicationStateFieldCoverage,
		assemblyline.WorkApplicationStateFieldName,
		assemblyline.WorkApplicationStateFieldKind,
		assemblyline.WorkApplicationRecordFieldCoverage,
		assemblyline.WorkApplicationRecordFieldName,
		assemblyline.WorkApplicationRecordFieldKind:
		return station.CodingServiceStateInterface, nil
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
	case assemblyline.WorkApplicationJobObjective,
		assemblyline.WorkApplicationBehaviorCoverage,
		assemblyline.WorkApplicationBehavior,
		assemblyline.WorkApplicationCriterionCoverage,
		assemblyline.WorkApplicationCriterion:
		return station.CodingWorkload, nil
	case assemblyline.WorkApplicationTargetTree:
		return station.CodingTargetTree, nil
	case assemblyline.WorkRepositorySearchAnchorCoverage,
		assemblyline.WorkRepositorySearchAnchor:
		return station.CodingRepositorySearchTerm, nil
	case assemblyline.WorkRepositoryChangeOwner:
		return station.CodingRepositoryChange, nil
	case assemblyline.WorkContextSearchTermCoverage,
		assemblyline.WorkContextSearchTerm:
		return station.ContextSearchTerms, nil
	case assemblyline.WorkContextRelevanceSelection:
		return station.ContextRelevance, nil
	case assemblyline.WorkContextMinification:
		return station.ContextMinification, nil
	case assemblyline.WorkConversationObjectiveKind:
		return station.ConversationObjectiveKind, nil
	case assemblyline.WorkConversationResponse,
		assemblyline.WorkRoleplayGroundedResponseText,
		assemblyline.WorkRoleplayGroundedResponseEvidenceRelation:
		return station.ConversationResponse, nil
	case assemblyline.WorkRoleplayCanonFactCoverage,
		assemblyline.WorkRoleplayCanonFact:
		return station.RoleplayCanonExtraction, nil
	case assemblyline.WorkRoleplayOngoingAction:
		return station.RoleplayOngoingAction, nil
	case assemblyline.WorkGroundedAnswerText,
		assemblyline.WorkGroundedAnswerEvidenceRelation:
		return station.GroundedAnswer, nil
	case assemblyline.WorkDatabaseSchemaSelectionCoverage,
		assemblyline.WorkDatabaseSchemaRelationSelection:
		return station.DatabaseSchemaSelection, nil
	case assemblyline.WorkDatabaseQueryFromRelation,
		assemblyline.WorkDatabaseQueryShape,
		assemblyline.WorkDatabaseQueryProjectionCoverage,
		assemblyline.WorkDatabaseQueryProjectionAggregate,
		assemblyline.WorkDatabaseQueryProjectionField,
		assemblyline.WorkDatabaseQueryProjectionTimeBucket,
		assemblyline.WorkDatabaseQueryFilterCoverage,
		assemblyline.WorkDatabaseQueryFilterField,
		assemblyline.WorkDatabaseQueryFilterOperator,
		assemblyline.WorkDatabaseQueryFilterValueCoverage,
		assemblyline.WorkDatabaseQueryFilterValue,
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
		assemblyline.WorkDatabaseQueryHavingValue,
		assemblyline.WorkDatabaseQueryOrderCoverage,
		assemblyline.WorkDatabaseQueryOrderProjection,
		assemblyline.WorkDatabaseQueryOrderDirection:
		return station.DatabaseQueryIntent, nil
	case assemblyline.WorkDatabaseEvidenceGap:
		return station.DatabaseEvidenceGap, nil
	case assemblyline.WorkDatabaseJoinPathSelection:
		return station.DatabaseJoinPathSelection, nil
	case assemblyline.WorkRepositoryEvidenceRelevanceLeaf:
		return station.RepositoryEvidenceRelevance, nil
	case assemblyline.WorkRepositoryGroundedIssueDetail,
		assemblyline.WorkRepositoryGroundedIssueKind:
		return station.RepositoryGroundedReview, nil
	case assemblyline.WorkRepositoryGroundedCorrection:
		return station.RepositoryGroundedCorrection, nil
	case assemblyline.WorkWebSearchTermCoverage,
		assemblyline.WorkWebSearchTerm:
		return station.WebSearchTerms, nil
	case assemblyline.WorkWebRelevanceRelation:
		return station.WebRelevance, nil
	case assemblyline.WorkWebSynthesisParagraphCoverage,
		assemblyline.WorkWebSynthesisParagraph,
		assemblyline.WorkWebSynthesisEvidenceRelation:
		return station.WebGroundedSynthesis, nil
	case assemblyline.WorkWebGroundedSynthesisCorrection:
		return station.WebGroundedSynthesisCorrection, nil
	case assemblyline.WorkWebReviewClaimCoverage,
		assemblyline.WorkWebReviewClaim,
		assemblyline.WorkWebReviewClaimVerdict,
		assemblyline.WorkWebReviewIssueEvidenceRelation,
		assemblyline.WorkWebReviewIssueDetail:
		return station.WebClaimEvidenceReview, nil
	case assemblyline.WorkArtifactHandling:
		return station.CodingArtifactHandling, nil
	case assemblyline.WorkKnownArtifactTruth:
		return station.CodingKnownArtifactTruth, nil
	case assemblyline.WorkDeclarationArtifactBoundary:
		return station.CodingDeclarationArtifactBoundary, nil
	case assemblyline.WorkArtifactCandidateSelection:
		return station.CodingArtifactCandidateSelection, nil
	case assemblyline.WorkCapabilityRelation:
		return station.CodingCapabilityRelation, nil
	case assemblyline.WorkSkillSelection:
		return station.CodingSkillSelection, nil
	case assemblyline.WorkTypeScriptRepairGuidance:
		return station.CodingFragmentRepairGuidance, nil
	case assemblyline.WorkFragmentGeneration, assemblyline.WorkFragmentModification:
		return station.CodingFragment, nil
	case assemblyline.WorkFragmentCorrection:
		return station.CodingFragmentCorrection, nil
	default:
		return "", fmt.Errorf("portable work kind %q is not a production semantic station", kind)
	}
}
