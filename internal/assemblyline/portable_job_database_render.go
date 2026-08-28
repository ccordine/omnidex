package assemblyline

import "fmt"

func renderPortableDatabaseLeaf(job PortableJob) (string, error) {
	switch job.Kind {
	case WorkDatabaseSchemaSelectionCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseSchemaSelectionCoveragePrompt)
	case WorkDatabaseSchemaRelationSelection:
		return renderPortableDatabaseInput(job, BuildDatabaseSchemaRelationSelectionPrompt)
	case WorkDatabaseQueryFromRelation:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFromRelationPrompt)
	case WorkDatabaseQueryShape:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryShapePrompt)
	case WorkDatabaseQueryProjectionCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionCoveragePrompt)
	case WorkDatabaseQueryProjectionAggregate:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionAggregatePrompt)
	case WorkDatabaseQueryProjectionField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionFieldPrompt)
	case WorkDatabaseQueryProjectionTimeBucket:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryProjectionTimeBucketPrompt)
	case WorkDatabaseQueryFilterCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterCoveragePrompt)
	case WorkDatabaseQueryFilterField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterFieldPrompt)
	case WorkDatabaseQueryFilterOperator:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterOperatorPrompt)
	case WorkDatabaseQueryFilterValueCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterValueCoveragePrompt)
	case WorkDatabaseQueryFilterValue:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryFilterValuePrompt)
	case WorkDatabaseQueryWindowCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowCoveragePrompt)
	case WorkDatabaseQueryWindowField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowFieldPrompt)
	case WorkDatabaseQueryWindowUnit:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowUnitPrompt)
	case WorkDatabaseQueryWindowAmount:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryWindowAmountPrompt)
	case WorkDatabaseQueryExistenceCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryExistenceCoveragePrompt)
	case WorkDatabaseQueryExistenceRelation:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryExistenceRelationPrompt)
	case WorkDatabaseQueryExistenceNegated:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryExistenceNegatedPrompt)
	case WorkDatabaseQueryHavingCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingCoveragePrompt)
	case WorkDatabaseQueryHavingAggregate:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingAggregatePrompt)
	case WorkDatabaseQueryHavingField:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingFieldPrompt)
	case WorkDatabaseQueryHavingOperator:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingOperatorPrompt)
	case WorkDatabaseQueryHavingValue:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryHavingValuePrompt)
	case WorkDatabaseQueryOrderCoverage:
		return renderPortableDatabaseInput(job, BuildDatabaseQueryOrderCoveragePrompt)
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
