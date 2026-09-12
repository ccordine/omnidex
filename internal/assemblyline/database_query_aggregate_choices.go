package assemblyline

import "github.com/gryph/omnidex/internal/datasource"

func databaseQueryProjectionAggregateChoices(
	input DatabaseQueryProjectionLeafInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	return databaseQueryAggregateChoices(input.State, []datasource.AggregateOperation{
		datasource.AggregateCountRows, datasource.AggregateCount, datasource.AggregateCountDistinct,
		datasource.AggregateSum, datasource.AggregateAverage, datasource.AggregateMinimum, datasource.AggregateMaximum,
	}, input.State.Shape != datasource.ResultScalar)
}

func databaseQueryHavingAggregateChoices(input DatabaseQueryHavingLeafInput) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	return databaseQueryAggregateChoices(input.State, []datasource.AggregateOperation{
		datasource.AggregateCountRows, datasource.AggregateCount, datasource.AggregateCountDistinct,
		datasource.AggregateSum, datasource.AggregateAverage,
	}, false)
}

// Field compatibility is known before aggregate selection. An operation with no
// eligible projected field is not an alternative for the model to consider.
func databaseQueryAggregateChoices(
	state DatabaseQueryIntentLeafState,
	operations []datasource.AggregateOperation,
	includeDirect bool,
) ([]OpaqueModelChoice, error) {
	specs := []databaseOpaqueChoiceSpec{}
	if includeDirect {
		specs = append(specs, databaseOpaqueChoiceSpec{
			"Select a field directly without aggregation", databaseQueryDirectProjectionChoice,
		})
	}
	for _, operation := range operations {
		if operation != datasource.AggregateCountRows && !databaseQueryHasAggregateField(state, operation) {
			continue
		}
		description, err := databaseQueryAggregateDescription(operation)
		if err != nil {
			return nil, err
		}
		specs = append(specs, databaseOpaqueChoiceSpec{description, string(operation)})
	}
	return databaseOpaqueChoices(specs)
}

func databaseQueryHasAggregateField(state DatabaseQueryIntentLeafState, operation datasource.AggregateOperation) bool {
	eligible := databaseQueryAggregateFieldEligible(operation)
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			if eligible(column) {
				return true
			}
		}
	}
	return false
}

func databaseQueryAggregateFieldEligible(
	aggregate datasource.AggregateOperation,
) func(datasource.IntentColumnProjection) bool {
	return func(column datasource.IntentColumnProjection) bool {
		switch aggregate {
		case "", datasource.AggregateCount, datasource.AggregateCountDistinct:
			return true
		case datasource.AggregateSum, datasource.AggregateAverage:
			return column.TypeCategory == datasource.TypeInteger || column.TypeCategory == datasource.TypeDecimal
		case datasource.AggregateMinimum, datasource.AggregateMaximum:
			if len(column.AllowedValues) > 0 {
				return false
			}
			switch column.TypeCategory {
			case datasource.TypeInteger, datasource.TypeDecimal, datasource.TypeText,
				datasource.TypeTemporal, datasource.TypeDate:
				return true
			}
		}
		return false
	}
}
