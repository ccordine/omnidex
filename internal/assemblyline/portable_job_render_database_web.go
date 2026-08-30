package assemblyline

func renderPortableDatabaseWebJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaRelationInventory, WorkDatabaseSchemaRelationNecessity,
		WorkDatabaseSchemaRelationResolution,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposeInventory, WorkDatabaseQueryPurposeNecessity,
		WorkDatabaseQueryPurposeRelation,
		WorkDatabaseQueryProjectionAggregate,
		WorkDatabaseQueryProjectionField, WorkDatabaseQueryProjectionTimeBucket,
		WorkDatabaseQueryFilterField, WorkDatabaseQueryFilterOperator,
		WorkDatabaseQueryFilterValue,
		WorkDatabaseQueryWindowField, WorkDatabaseQueryWindowUnit,
		WorkDatabaseQueryWindowAmount,
		WorkDatabaseQueryExistenceRelation, WorkDatabaseQueryExistenceNegated,
		WorkDatabaseQueryHavingAggregate,
		WorkDatabaseQueryHavingField, WorkDatabaseQueryHavingOperator,
		WorkDatabaseQueryHavingValue,
		WorkDatabaseQueryOrderProjection, WorkDatabaseQueryOrderDirection:
		return handledPortableRender(renderPortableDatabaseLeaf(job))
	case WorkDatabaseJoinPathSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildDatabaseJoinPathSelectionPrompt))
	case WorkWebRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebRelevanceRelationPrompt))
	case WorkWebSynthesisParagraphInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphInventoryPrompt))
	case WorkWebSynthesisEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisEvidenceRelationPrompt))
	case WorkWebSynthesisParagraphAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildWebSynthesisParagraphAuthorizationPrompt))
	default:
		return "", false, nil
	}
}
