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
	case WorkWebRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebRelevanceRelationPrompt))
	case WorkWebSynthesisParagraphCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphCoveragePrompt))
	case WorkWebSynthesisParagraph:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphPrompt))
	case WorkWebSynthesisEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisEvidenceRelationPrompt))
	default:
		return "", false, nil
	}
}
