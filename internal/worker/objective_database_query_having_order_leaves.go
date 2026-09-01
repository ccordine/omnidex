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
		datasource.MaxIntentGroups-len(state.Having), false, call, total,
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
			fieldID, resolved, err := assemblyline.ResolveSoleDatabaseQueryHavingFieldLeaf(leaf)
			if err != nil {
				return state, total, err
			}
			if resolved {
				leaf.FieldID = fieldID
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
		datasource.MaxIntentOrderTerms-len(state.OrderBy),
		state.Shape == datasource.ResultRanking && len(state.OrderBy) == 0,
		call, total,
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
		var projection int
		resolvedProjection, resolved, err := assemblyline.ResolveSoleDatabaseQueryOrderProjectionLeaf(leaf)
		if err != nil {
			return state, total, err
		}
		if resolved {
			projection = resolvedProjection
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
