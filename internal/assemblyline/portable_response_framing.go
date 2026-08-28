package assemblyline

import "fmt"

// PortableResponseFraming is code-owned authority for provider termination.
// It describes only the byte grammar of one result, never its meaning.
type PortableResponseFraming string

const (
	PortableResponseFramingSingleLine       PortableResponseFraming = "single_line"
	PortableResponseFramingNaturalMultiline PortableResponseFraming = "natural_multiline"
	portableResponseFramingOriginal         PortableResponseFraming = "retained_original"
)

// PortableResponseFramingForWorkKind classifies every registered work kind by
// its exact result grammar. Response correction is resolved from its retained
// original job by PortableResponseFramingForJob.
func PortableResponseFramingForWorkKind(
	kind WorkKind,
) (PortableResponseFraming, error) {
	switch kind {
	case WorkApplicationProductContext,
		WorkApplicationRequirement,
		WorkApplicationTargetTree,
		WorkRepositoryRequirement,
		WorkRepositoryGroundedCorrection,
		WorkContextMinification,
		WorkConversationResponse,
		WorkRoleplayGroundedResponseText,
		WorkRoleplayOngoingAction,
		WorkGroundedAnswerText,
		WorkDatabaseEvidenceGap,
		WorkWebSynthesisParagraph,
		WorkWebGroundedSynthesisCorrection,
		WorkTypeScriptRepairGuidance,
		WorkFragmentGeneration,
		WorkFragmentModification,
		WorkFragmentCorrection:
		return PortableResponseFramingNaturalMultiline, nil
	case WorkResponseCorrection:
		return portableResponseFramingOriginal, nil
	case WorkApplicationContextNeedCoverage,
		WorkApplicationContextNeedQuestion,
		WorkApplicationRequirementCoverage,
		WorkApplicationProjectStackConstraint,
		WorkApplicationServiceContinuedAvailability,
		WorkApplicationServicePersistenceDestination,
		WorkApplicationServiceStateLifetime,
		WorkApplicationStateFieldCoverage,
		WorkApplicationStateFieldName,
		WorkApplicationStateFieldKind,
		WorkApplicationRecordFieldCoverage,
		WorkApplicationRecordFieldName,
		WorkApplicationRecordFieldKind,
		WorkApplicationServiceEndpointRequirement,
		WorkApplicationServiceEndpointExposure,
		WorkApplicationServiceEndpointMethod,
		WorkApplicationServiceEndpointRouteTemplate,
		WorkApplicationServiceEndpointRequestMedia,
		WorkApplicationServiceEndpointResponseMedia,
		WorkApplicationServiceEndpointSuccessStatus,
		WorkApplicationClassify,
		WorkApplicationJobObjective,
		WorkApplicationBehaviorCoverage,
		WorkApplicationBehavior,
		WorkApplicationCriterionCoverage,
		WorkApplicationCriterion,
		WorkRepositoryRequirementCoverage,
		WorkRepositorySearchAnchorCoverage,
		WorkRepositorySearchAnchor,
		WorkRepositoryEvidenceRelevanceLeaf,
		WorkRepositoryChangeOwner,
		WorkRepositoryGroundedIssueDetail,
		WorkRepositoryGroundedIssueKind,
		WorkContextSearchTermCoverage,
		WorkContextSearchTerm,
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
		WorkWebSearchTermCoverage,
		WorkWebSearchTerm,
		WorkWebRelevanceRelation,
		WorkWebSynthesisParagraphCoverage,
		WorkWebSynthesisEvidenceRelation,
		WorkWebReviewClaimCoverage,
		WorkWebReviewClaim,
		WorkWebReviewClaimVerdict,
		WorkWebReviewIssueEvidenceRelation,
		WorkWebReviewIssueDetail,
		WorkArtifactHandling,
		WorkKnownArtifactTruth,
		WorkDeclarationArtifactBoundary,
		WorkArtifactCandidateSelection,
		WorkCapabilityRelation,
		WorkSkillSelection:
		return PortableResponseFramingSingleLine, nil
	default:
		return "", fmt.Errorf("portable work kind %q has no registered response framing", kind)
	}
}

// PortableResponseFramingForJob resolves inherited correction framing and
// returns only a provider-actionable framing value.
func PortableResponseFramingForJob(job PortableJob) (PortableResponseFraming, error) {
	if err := job.Validate(); err != nil {
		return "", err
	}
	framing, err := PortableResponseFramingForWorkKind(job.Kind)
	if err != nil {
		return "", err
	}
	if framing != portableResponseFramingOriginal {
		return framing, nil
	}
	var input ResponseCorrectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return "", fmt.Errorf("decode response correction framing authority: %w", err)
	}
	originalFraming, err := PortableResponseFramingForWorkKind(input.Original.Kind)
	if err != nil {
		return "", err
	}
	if originalFraming == portableResponseFramingOriginal {
		return "", fmt.Errorf("response correction framing cannot inherit another correction")
	}
	return originalFraming, nil
}
