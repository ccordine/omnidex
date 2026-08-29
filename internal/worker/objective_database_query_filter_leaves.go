package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryFilters(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	scopeRelationID string,
	accepted []datasource.RelationalPredicate,
	call objectiveDatabaseRawLeafCall,
	total int,
) ([]datasource.RelationalPredicate, int, error) {
	for {
		leaf := assemblyline.DatabaseQueryFilterLeafInput{
			State: state, ScopeRelationID: scopeRelationID,
			AcceptedFilters: append([]datasource.RelationalPredicate{}, accepted...),
			AcceptedValues:  []datasource.IntentLiteral{},
		}
		coverageJob, err := assemblyline.NewDatabaseQueryFilterCoverageJob(leaf)
		if err != nil {
			return nil, total, err
		}
		coverage, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_filter_coverage", coverageJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseQueryFilterCoverageLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return nil, total, err
		}
		if coverage == assemblyline.DatabaseQueryNoUncoveredItem {
			return accepted, total, nil
		}
		fields := objectiveDatabaseFilterFields(state, scopeRelationID)
		if len(fields) == 0 {
			return nil, total, fmt.Errorf("database query filter has no compatible field")
		}
		if len(fields) == 1 {
			leaf.FieldID = fields[0]
		} else {
			job, err := assemblyline.NewDatabaseQueryFilterFieldJob(leaf)
			if err != nil {
				return nil, total, err
			}
			leaf.FieldID, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_filter_field", job,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryFilterFieldLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return nil, total, err
			}
		}
		operatorJob, err := assemblyline.NewDatabaseQueryFilterOperatorJob(leaf)
		if err != nil {
			return nil, total, err
		}
		leaf.Operator, calls, err = callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_filter_operator", operatorJob,
			func(raw string) (datasource.FilterOperator, error) {
				return assemblyline.DecodeDatabaseQueryFilterOperatorLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return nil, total, err
		}
		values := []datasource.IntentLiteral{}
		switch leaf.Operator {
		case datasource.FilterIsNull, datasource.FilterIsNotNull:
		case datasource.FilterIn, datasource.FilterNotIn:
			values, total, err = resolveDatabaseQueryFilterValues(ctx, leaf, call, total)
		default:
			leaf.AcceptedValues = values
			valueJob, jobErr := assemblyline.NewDatabaseQueryFilterValueJob(leaf)
			if jobErr != nil {
				return nil, total, jobErr
			}
			var value datasource.IntentLiteral
			value, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_filter_value", valueJob,
				func(raw string) (datasource.IntentLiteral, error) {
					return assemblyline.DecodeDatabaseQueryFilterValueLeaf(leaf, raw)
				},
			)
			total += calls
			values = append(values, value)
		}
		if err != nil {
			return nil, total, err
		}
		accepted = append(accepted, datasource.RelationalPredicate{
			FieldID: leaf.FieldID, Operator: leaf.Operator, Values: values,
		})
	}
}

func resolveDatabaseQueryFilterValues(
	ctx context.Context,
	leaf assemblyline.DatabaseQueryFilterLeafInput,
	call objectiveDatabaseRawLeafCall,
	total int,
) ([]datasource.IntentLiteral, int, error) {
	values := []datasource.IntentLiteral{}
	for {
		leaf.AcceptedValues = append([]datasource.IntentLiteral{}, values...)
		if len(values) > 0 {
			coverageJob, err := assemblyline.NewDatabaseQueryFilterValueCoverageJob(leaf)
			if err != nil {
				return nil, total, err
			}
			coverage, calls, err := callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_filter_value_coverage", coverageJob,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryFilterValueCoverageLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return nil, total, err
			}
			if coverage == assemblyline.DatabaseQueryNoUncoveredValue {
				return values, total, nil
			}
		}
		valueJob, err := assemblyline.NewDatabaseQueryFilterValueJob(leaf)
		if err != nil {
			return nil, total, err
		}
		value, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_filter_value", valueJob,
			func(raw string) (datasource.IntentLiteral, error) {
				return assemblyline.DecodeDatabaseQueryFilterValueLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return nil, total, err
		}
		values = append(values, value)
	}
}

func objectiveDatabaseFilterFields(
	state assemblyline.DatabaseQueryIntentLeafState,
	scopeRelationID string,
) []string {
	fields := []string{}
	for _, relation := range state.Authority.SchemaProjection.Relations {
		if scopeRelationID != "" && relation.ID != scopeRelationID {
			continue
		}
		for _, column := range relation.Columns {
			fields = append(fields, column.ID)
		}
	}
	return fields
}
