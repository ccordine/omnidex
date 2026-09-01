package assemblyline

func renderPortableDatabaseWebJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaRelationChoice,
		WorkDatabaseQueryFromRelation, WorkDatabaseQueryShape,
		WorkDatabaseQueryPurposePresence,
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
	default:
		return "", false, nil
	}
}
