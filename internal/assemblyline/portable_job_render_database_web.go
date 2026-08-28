package assemblyline

func renderPortableDatabaseWebJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaSelectionCoverage, WorkDatabaseSchemaRelationSelection,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryProjectionCoverage, WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterCoverage, WorkDatabaseQueryFilterField,
		WorkDatabaseQueryFilterOperator, WorkDatabaseQueryFilterValueCoverage,
		WorkDatabaseQueryFilterValue, WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount, WorkDatabaseQueryExistenceCoverage,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingCoverage, WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue, WorkDatabaseQueryOrderCoverage,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection:
		return handledPortableRender(renderPortableDatabaseLeaf(job))
	case WorkDatabaseEvidenceGap:
		return handledPortableRender(renderDecodedPortableInput(job, BuildDatabaseEvidenceGapPrompt))
	case WorkDatabaseJoinPathSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildDatabaseJoinPathSelectionPrompt))
	case WorkWebSearchTermCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSearchTermCoveragePrompt))
	case WorkWebSearchTerm:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSearchTermPrompt))
	case WorkWebRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebRelevanceRelationPrompt))
	case WorkWebSynthesisParagraphCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphCoveragePrompt))
	case WorkWebSynthesisParagraph:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphPrompt))
	case WorkWebSynthesisEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisEvidenceRelationPrompt))
	case WorkWebGroundedSynthesisCorrection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebGroundedSynthesisCorrectionPrompt))
	case WorkWebReviewClaimCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebReviewClaimCoveragePrompt))
	case WorkWebReviewClaim:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebReviewClaimPrompt))
	case WorkWebReviewClaimVerdict:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebReviewClaimVerdictPrompt))
	case WorkWebReviewIssueEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebReviewIssueEvidenceRelationPrompt))
	case WorkWebReviewIssueDetail:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebReviewIssueDetailPrompt))
	default:
		return "", false, nil
	}
}
