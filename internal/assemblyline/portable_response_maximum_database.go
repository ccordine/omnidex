package assemblyline

import (
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func portableDatabaseResponseMaximum(job PortableJob) (portableResponseContract, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaRelationChoice:
		maximum, err := databaseSchemaRelationChoiceMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFromRelation:
		maximum, err := databaseFromRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryShape:
		maximum, err := opaqueModelChoiceBuilderResponseContract(databaseQueryShapeChoices)
		return maximum, true, err
	case WorkDatabaseQueryPurposeInventory:
		return portableResponseContract{maximum: maxDatabaseQueryPurposeInventoryBytes}, true, nil
	case WorkDatabaseQueryPurposeNecessity:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			databaseQueryPurposeNecessityChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryPurposeRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			databaseQueryPurposeRelationChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryProjectionAggregate:
		var input DatabaseQueryProjectionLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return databaseQueryProjectionAggregateChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryProjectionField:
		maximum, err := databaseProjectionFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryProjectionTimeBucket:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
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
	case WorkDatabaseQueryFilterValueChoice:
		var input DatabaseQueryFilterLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return databaseQueryFilterValueSubsetChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryWindowField:
		maximum, err := databaseWindowFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryWindowUnit:
		var input DatabaseQueryWindowLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return databaseQueryWindowUnitChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryWindowAmount:
		return portableResponseContract{maximum: len(strconv.Itoa(10000))}, true, nil
	case WorkDatabaseQueryExistenceRelation:
		maximum, err := databaseExistenceRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryExistenceNegated:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			databaseQueryExistenceNegatedChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryHavingAggregate:
		var input DatabaseQueryHavingLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return databaseQueryHavingAggregateChoices(input)
		})
		return maximum, true, err
	case WorkDatabaseQueryHavingField:
		maximum, err := databaseHavingFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryHavingOperator:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			databaseQueryHavingOperatorChoices,
		)
		return maximum, true, err
	case WorkDatabaseQueryHavingValue:
		return portableResponseContract{maximum: datasource.MaxIntentDecimalLiteralBytes}, true, nil
	case WorkDatabaseQueryOrderProjection:
		maximum, err := databaseOrderProjectionMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryOrderDirection:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			databaseQueryOrderDirectionChoices,
		)
		return maximum, true, err
	case WorkDatabaseJoinPathSelection:
		maximum, err := databaseJoinPathMaximum(job)
		return maximum, true, err
	default:
		return portableResponseContract{}, false, nil
	}
}

func databaseSchemaRelationChoiceMaximum(job PortableJob) (portableResponseContract, error) {
	var input DatabaseSchemaRelationChoiceInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return portableResponseContract{}, err
	}
	return opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
		return databaseSchemaRelationChoices(input)
	})
}

func databaseFromRelationMaximum(job PortableJob) (portableResponseContract, error) {
	var state DatabaseQueryIntentLeafState
	if err := decodePortablePayload(job.Payload, &state); err != nil {
		return portableResponseContract{}, err
	}
	return opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFromRelationChoices(state)
	})
}

func databaseProjectionFieldMaximum(job PortableJob) (portableResponseContract, error) {
	var input DatabaseQueryProjectionLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return portableResponseContract{}, err
	}
	return opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFieldChoices(
			input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
		)
	})
}
