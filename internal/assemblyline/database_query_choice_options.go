package assemblyline

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

const (
	databaseQueryDirectProjectionChoice = "code-owned:direct-projection"
	databaseQueryNoTimeBucketChoice     = "code-owned:no-time-bucket"
)

type databaseOpaqueChoiceSpec struct {
	description string
	value       string
}

func databaseOpaqueChoices(specs []databaseOpaqueChoiceSpec) ([]OpaqueModelChoice, error) {
	if len(specs) == 0 {
		return []OpaqueModelChoice{}, nil
	}
	choices := make([]OpaqueModelChoice, 0, len(specs))
	for index, spec := range specs {
		choice, err := NewOpaqueModelChoice(spec.description, spec.value)
		if err != nil {
			return nil, fmt.Errorf("database choice %d: %w", index, err)
		}
		choices = append(choices, choice)
	}
	if err := validateOpaqueModelChoices(choices); err != nil {
		return nil, err
	}
	return choices, nil
}

func resolveSoleDatabaseOpaqueChoice(choices []OpaqueModelChoice) (string, bool, error) {
	if len(choices) == 0 {
		return "", false, nil
	}
	return ResolveSoleOpaqueModelChoice(choices)
}

func databaseQueryFromRelationChoices(
	state DatabaseQueryIntentLeafState,
) ([]OpaqueModelChoice, error) {
	if err := state.validate(); err != nil {
		return nil, err
	}
	return databaseQueryRelationChoicesExcluding(state, nil)
}

func databaseQueryRelationChoicesExcluding(
	state DatabaseQueryIntentLeafState,
	excluded map[string]struct{},
) ([]OpaqueModelChoice, error) {
	specs := make([]databaseOpaqueChoiceSpec, 0, len(state.Authority.SchemaProjection.Relations))
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if _, skip := excluded[relation.ID]; skip {
			continue
		}
		description := fmt.Sprintf("%s.%s (%s)", relation.SchemaName, relation.Name, relation.Kind)
		if len(relation.Columns) > 0 {
			fields := make([]string, 0, len(relation.Columns))
			for _, column := range relation.Columns {
				fields = append(fields, fmt.Sprintf("%s (%s)", column.Name, column.TypeCategory))
			}
			description += "; fields: " + strings.Join(fields, ", ")
		}
		specs = append(specs, databaseOpaqueChoiceSpec{description, relation.ID})
	}
	return databaseOpaqueChoices(specs)
}

func databaseQueryShapeChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Matching records with requested fields", string(datasource.ResultRecords)},
		{"One aggregate value", string(datasource.ResultScalar)},
		{"Records ordered by a requested measure", string(datasource.ResultRanking)},
		{"Values grouped into a distribution", string(datasource.ResultDistribution)},
		{"Requested values compared across groups", string(datasource.ResultComparison)},
		{"Requested values over time", string(datasource.ResultTrend)},
		{"Whether matching records exist", string(datasource.ResultExistence)},
	})
}

func databaseQueryProjectionAggregateChoices(
	input DatabaseQueryProjectionLeafInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	specs := []databaseOpaqueChoiceSpec{}
	if input.State.Shape != datasource.ResultScalar {
		specs = append(specs, databaseOpaqueChoiceSpec{
			"Select a field directly without aggregation", databaseQueryDirectProjectionChoice,
		})
	}
	specs = append(specs,
		databaseOpaqueChoiceSpec{"Count matching rows", string(datasource.AggregateCountRows)},
		databaseOpaqueChoiceSpec{"Count non-null values in one field", string(datasource.AggregateCount)},
		databaseOpaqueChoiceSpec{"Count distinct non-null values in one field", string(datasource.AggregateCountDistinct)},
		databaseOpaqueChoiceSpec{"Sum numeric values in one field", string(datasource.AggregateSum)},
		databaseOpaqueChoiceSpec{"Average numeric values in one field", string(datasource.AggregateAverage)},
		databaseOpaqueChoiceSpec{"Minimum value in one field", string(datasource.AggregateMinimum)},
		databaseOpaqueChoiceSpec{"Maximum value in one field", string(datasource.AggregateMaximum)},
	)
	return databaseOpaqueChoices(specs)
}

func databaseQueryFieldChoices(
	state DatabaseQueryIntentLeafState,
	relationID string,
	eligible func(datasource.IntentColumnProjection) bool,
) ([]OpaqueModelChoice, error) {
	specs := []databaseOpaqueChoiceSpec{}
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if relationID != "" && relation.ID != relationID {
			continue
		}
		for _, column := range relation.Columns {
			if eligible != nil && !eligible(column) {
				continue
			}
			description := fmt.Sprintf(
				"%s.%s.%s (%s; nullable: %t)",
				relation.SchemaName, relation.Name, column.Name, column.TypeCategory, column.Nullable,
			)
			if len(column.AllowedValues) > 0 {
				description += "; allowed values: " + strings.Join(column.AllowedValues, ", ")
			}
			specs = append(specs, databaseOpaqueChoiceSpec{description, column.ID})
		}
	}
	return databaseOpaqueChoices(specs)
}

func databaseQueryProjectionTimeBucketChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Use the temporal field directly", databaseQueryNoTimeBucketChoice},
		{"Group timestamps by calendar day", string(datasource.BucketDay)},
		{"Group timestamps by calendar week", string(datasource.BucketWeek)},
		{"Group timestamps by calendar month", string(datasource.BucketMonth)},
		{"Group timestamps by calendar quarter", string(datasource.BucketQuarter)},
		{"Group timestamps by calendar year", string(datasource.BucketYear)},
	})
}

func databaseQueryFilterOperatorChoices(
	input DatabaseQueryFilterLeafInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validateField(); err != nil {
		return nil, err
	}
	operators := databaseQueryFilterOperators(input.State, input.FieldID)
	specs := make([]databaseOpaqueChoiceSpec, 0, len(operators))
	for _, operator := range operators {
		description, err := databaseQueryFilterOperatorDescription(operator)
		if err != nil {
			return nil, err
		}
		specs = append(specs, databaseOpaqueChoiceSpec{description, string(operator)})
	}
	return databaseOpaqueChoices(specs)
}

func databaseQueryFilterValueChoices(
	input DatabaseQueryFilterLeafInput,
) ([]OpaqueModelChoice, bool, error) {
	if err := input.validateOperator(); err != nil {
		return nil, false, err
	}
	column, _, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok {
		return nil, false, fmt.Errorf("database query filter field %q was not projected", input.FieldID)
	}
	values := append([]string(nil), column.AllowedValues...)
	closed := len(values) > 0
	if column.TypeCategory == datasource.TypeBoolean && !closed {
		values = []string{"true", "false"}
		closed = true
	}
	if !closed {
		return nil, false, nil
	}
	accepted := make(map[string]struct{}, len(input.AcceptedValues))
	for _, literal := range input.AcceptedValues {
		accepted[literal.Value] = struct{}{}
	}
	specs := make([]databaseOpaqueChoiceSpec, 0, len(values))
	for _, value := range values {
		if _, exists := accepted[value]; exists {
			continue
		}
		specs = append(specs, databaseOpaqueChoiceSpec{
			description: "The exact value " + strconv.Quote(value),
			value:       value,
		})
	}
	choices, err := databaseOpaqueChoices(specs)
	return choices, true, err
}

func databaseQueryWindowUnitChoices(input DatabaseQueryWindowLeafInput) ([]OpaqueModelChoice, error) {
	if err := input.validateField(); err != nil {
		return nil, err
	}
	column, _, _ := databaseQueryColumn(input.State, input.FieldID)
	specs := []databaseOpaqueChoiceSpec{}
	if column.TypeCategory != datasource.TypeDate {
		specs = append(specs, databaseOpaqueChoiceSpec{"Hours before the authoritative instant", string(datasource.WindowHour)})
	}
	specs = append(specs,
		databaseOpaqueChoiceSpec{"Days before the authoritative instant", string(datasource.WindowDay)},
		databaseOpaqueChoiceSpec{"Weeks before the authoritative instant", string(datasource.WindowWeek)},
		databaseOpaqueChoiceSpec{"Months before the authoritative instant", string(datasource.WindowMonth)},
		databaseOpaqueChoiceSpec{"Years before the authoritative instant", string(datasource.WindowYear)},
	)
	return databaseOpaqueChoices(specs)
}

func databaseQueryExistenceNegatedChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Matching rows in the focused relation must exist", "false"},
		{"Matching rows in the focused relation must not exist", "true"},
	})
}

func databaseQueryHavingAggregateChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Count matching rows", string(datasource.AggregateCountRows)},
		{"Count non-null values in one field", string(datasource.AggregateCount)},
		{"Count distinct non-null values in one field", string(datasource.AggregateCountDistinct)},
		{"Sum numeric values in one field", string(datasource.AggregateSum)},
		{"Average numeric values in one field", string(datasource.AggregateAverage)},
	})
}

func databaseQueryHavingOperatorChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Equal to the requested value", string(datasource.FilterEqual)},
		{"Not equal to the requested value", string(datasource.FilterNotEqual)},
		{"Greater than the requested value", string(datasource.FilterGT)},
		{"Greater than or equal to the requested value", string(datasource.FilterGTE)},
		{"Less than the requested value", string(datasource.FilterLT)},
		{"Less than or equal to the requested value", string(datasource.FilterLTE)},
	})
}

func databaseQueryOrderProjectionChoices(
	input DatabaseQueryOrderLeafInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	used := make(map[int]struct{}, len(input.State.OrderBy))
	for _, term := range input.State.OrderBy {
		used[term.Projection] = struct{}{}
	}
	specs := []databaseOpaqueChoiceSpec{}
	for index, projection := range input.State.Projections {
		if _, exists := used[index]; exists {
			continue
		}
		description, err := databaseQueryProjectionSemantic(input.State, projection)
		if err != nil {
			return nil, err
		}
		specs = append(specs, databaseOpaqueChoiceSpec{description, strconv.Itoa(index)})
	}
	return databaseOpaqueChoices(specs)
}

func databaseQueryOrderDirectionChoices() ([]OpaqueModelChoice, error) {
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{"Ascending: smaller or earlier values first", string(datasource.OrderAscending)},
		{"Descending: larger or later values first", string(datasource.OrderDescending)},
	})
}
