package assemblyline

import (
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func portableDatabaseResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaRelationChoice:
		maximum, err := databaseSchemaRelationChoiceMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFromRelation:
		maximum, err := databaseFromRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryShape:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(databaseQueryShapeChoices)
		return maximum, true, err
	case WorkDatabaseQueryPurposePresence:
		var input DatabaseQueryPurposeAuthority
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return databaseQueryPurposePresenceChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryPurposeInventory:
		return maxDatabaseQueryPurposeInventoryBytes, true, nil
	case WorkDatabaseQueryPurposeNecessity:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryPurposeNecessityChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryPurposeRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryPurposeRelationChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryProjectionAggregate:
		var input DatabaseQueryProjectionLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return databaseQueryProjectionAggregateChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryProjectionField:
		maximum, err := databaseProjectionFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryProjectionTimeBucket:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryProjectionTimeBucketChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryFilterField:
		maximum, err := databaseFilterFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFilterOperator:
		maximum, err := databaseFilterOperatorMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFilterValue:
		maximum, err := databaseFilterValueMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryWindowField:
		maximum, err := databaseWindowFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryWindowUnit:
		var input DatabaseQueryWindowLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return databaseQueryWindowUnitChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryWindowAmount:
		return len(strconv.Itoa(10000)), true, nil
	case WorkDatabaseQueryExistenceRelation:
		maximum, err := databaseExistenceRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryExistenceNegated:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryExistenceNegatedChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryHavingAggregate:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryHavingAggregateChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryHavingField:
		maximum, err := databaseHavingFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryHavingOperator:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryHavingOperatorChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryHavingValue:
		return datasource.MaxIntentDecimalLiteralBytes, true, nil
	case WorkDatabaseQueryOrderProjection:
		maximum, err := databaseOrderProjectionMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryOrderDirection:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			databaseQueryOrderDirectionChoices,
		)
		return maximum, true, err
	case WorkDatabaseJoinPathSelection:
		maximum, err := databaseJoinPathMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}

func databaseSchemaRelationChoiceMaximum(job PortableJob) (int, error) {
	var input DatabaseSchemaRelationChoiceInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseSchemaRelationChoices(input)
	})
}

func databaseFromRelationMaximum(job PortableJob) (int, error) {
	var state DatabaseQueryIntentLeafState
	if err := decodePortablePayload(job.Payload, &state); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFromRelationChoices(state)
	})
}

func databaseProjectionFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryProjectionLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFieldChoices(
			input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
		)
	})
}
