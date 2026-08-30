package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryHaving(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	purposes, nextTotal, err := resolveDatabaseQueryPurposeQueue(
		ctx,
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryHavingPurpose,
		},
		datasource.MaxIntentGroups-len(state.Having), call, total,
	)
	total = nextTotal
	if err != nil {
		return state, total, err
	}
	for _, purpose := range purposes {
		leaf := assemblyline.DatabaseQueryHavingLeafInput{State: state, Purpose: purpose}
		var calls int
		aggregateJob, err := assemblyline.NewDatabaseQueryHavingAggregateJob(leaf)
		if err != nil {
			return state, total, err
		}
		leaf.Aggregate, calls, err = callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_having_aggregate", aggregateJob,
			func(raw string) (datasource.AggregateOperation, error) {
				return assemblyline.DecodeDatabaseQueryHavingAggregateLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		if leaf.Aggregate != datasource.AggregateCountRows {
			fields := objectiveDatabaseProjectionFields(state, leaf.Aggregate)
			if len(fields) == 0 {
				return state, total, fmt.Errorf("database query having aggregate has no compatible field")
			}
			if len(fields) == 1 {
				leaf.FieldID = fields[0]
			} else {
				fieldJob, err := assemblyline.NewDatabaseQueryHavingFieldJob(leaf)
				if err != nil {
					return state, total, err
				}
				leaf.FieldID, calls, err = callObjectiveDatabaseRawLeaf(
					ctx, call, "database_query_having_field", fieldJob,
					func(raw string) (string, error) {
						return assemblyline.DecodeDatabaseQueryHavingFieldLeaf(leaf, raw)
					},
				)
				total += calls
				if err != nil {
					return state, total, err
				}
			}
		}
		operatorJob, err := assemblyline.NewDatabaseQueryHavingOperatorJob(leaf)
		if err != nil {
			return state, total, err
		}
		leaf.Operator, calls, err = callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_having_operator", operatorJob,
			func(raw string) (datasource.FilterOperator, error) {
				return assemblyline.DecodeDatabaseQueryHavingOperatorLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		valueJob, err := assemblyline.NewDatabaseQueryHavingValueJob(leaf)
		if err != nil {
			return state, total, err
		}
		value, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_having_value", valueJob,
			func(raw string) (datasource.IntentLiteral, error) {
				return assemblyline.DecodeDatabaseQueryHavingValueLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		state.Having = append(state.Having, datasource.AggregatePredicate{
			Aggregate: leaf.Aggregate, FieldID: leaf.FieldID,
			Operator: leaf.Operator, Value: value,
		})
	}
	return state, total, nil
}

func resolveDatabaseQueryOrder(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	purposes, nextTotal, err := resolveDatabaseQueryPurposeQueue(
		ctx,
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryOrderPurpose,
		},
		datasource.MaxIntentOrderTerms-len(state.OrderBy), call, total,
	)
	total = nextTotal
	if err != nil {
		return state, total, err
	}
	if state.Shape == datasource.ResultRanking && len(state.OrderBy) == 0 && len(purposes) == 0 {
		return state, total, fmt.Errorf("database query ranking shape requires one accepted ordering purpose")
	}
	for _, purpose := range purposes {
		leaf := assemblyline.DatabaseQueryOrderLeafInput{State: state, Purpose: purpose}
		var calls int
		remaining := objectiveDatabaseOrderProjections(state)
		if len(remaining) == 0 {
			return state, total, fmt.Errorf("database query order has no unused projection")
		}
		var projection int
		if len(remaining) == 1 {
			projection = remaining[0]
		} else {
			projectionJob, err := assemblyline.NewDatabaseQueryOrderProjectionJob(leaf)
			if err != nil {
				return state, total, err
			}
			projection, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_order_projection", projectionJob,
				func(raw string) (int, error) {
					return assemblyline.DecodeDatabaseQueryOrderProjectionLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		leaf.Projection = &projection
		directionJob, err := assemblyline.NewDatabaseQueryOrderDirectionJob(leaf)
		if err != nil {
			return state, total, err
		}
		direction, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_order_direction", directionJob,
			func(raw string) (datasource.OrderDirection, error) {
				return assemblyline.DecodeDatabaseQueryOrderDirectionLeaf(leaf, raw)
			},
		)
		total += calls
		if err != nil {
			return state, total, err
		}
		state.OrderBy = append(state.OrderBy, datasource.OrderTerm{
			Projection: projection, Direction: direction,
		})
	}
	return state, total, nil
}

func objectiveDatabaseOrderProjections(state assemblyline.DatabaseQueryIntentLeafState) []int {
	used := map[int]struct{}{}
	for _, term := range state.OrderBy {
		used[term.Projection] = struct{}{}
	}
	projections := []int{}
	for index := range state.Projections {
		if _, duplicate := used[index]; !duplicate {
			projections = append(projections, index)
		}
	}
	return projections
}
