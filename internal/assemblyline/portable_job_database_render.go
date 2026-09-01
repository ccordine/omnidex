package assemblyline

import "fmt"

func renderPortableDatabaseLeaf(job PortableJob) (string, error) {
	switch job.Kind {
	case WorkDatabaseSchemaRelationChoice:
		return renderPortableDatabaseInput(job, BuildDatabaseSchemaRelationChoicePrompt)
	case WorkDatabaseQueryFromRelation:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFromRelationPrompt)
	case WorkDatabaseQueryShape:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryShapePrompt)
	case WorkDatabaseQueryPurposePresence:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryPurposePresencePrompt)
	case WorkDatabaseQueryPurposeInventory:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryPurposeInventoryPrompt)
	case WorkDatabaseQueryPurposeNecessity:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryPurposeNecessityPrompt)
	case WorkDatabaseQueryPurposeRelation:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryPurposeRelationPrompt)
	case WorkDatabaseQueryProjectionAggregate:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionAggregatePrompt)
	case WorkDatabaseQueryProjectionField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionFieldPrompt)
	case WorkDatabaseQueryProjectionTimeBucket:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionTimeBucketPrompt)
	case WorkDatabaseQueryFilterField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterFieldPrompt)
	case WorkDatabaseQueryFilterOperator:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterOperatorPrompt)
	case WorkDatabaseQueryFilterValue:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterValuePrompt)
	case WorkDatabaseQueryWindowField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowFieldPrompt)
	case WorkDatabaseQueryWindowUnit:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowUnitPrompt)
	case WorkDatabaseQueryWindowAmount:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowAmountPrompt)
	case WorkDatabaseQueryExistenceRelation:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryExistenceRelationPrompt)
	case WorkDatabaseQueryExistenceNegated:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryExistenceNegatedPrompt)
	case WorkDatabaseQueryHavingAggregate:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingAggregatePrompt)
	case WorkDatabaseQueryHavingField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingFieldPrompt)
	case WorkDatabaseQueryHavingOperator:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingOperatorPrompt)
	case WorkDatabaseQueryHavingValue:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingValuePrompt)
	case WorkDatabaseQueryOrderProjection:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryOrderProjectionPrompt)
	case WorkDatabaseQueryOrderDirection:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryOrderDirectionPrompt)
	default:
		return "", fmt.Errorf("portable database leaf kind %q is not registered", job.Kind)
	}
}

func renderPortableDatabaseInput[T any](
	job PortableJob,
	build func(T) (string, error),
) (string, error) {
	var input T
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return "", err
	}
	return build(input)
}
