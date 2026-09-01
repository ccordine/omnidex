package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func resolveDatabaseQueryProjections(
	ctx context.Context,
	state assemblyline.DatabaseQueryIntentLeafState,
	call objectiveDatabaseRawLeafCall,
	total int,
) (assemblyline.DatabaseQueryIntentLeafState, int, error) {
	maximumPurposes := datasource.MaxIntentProjections - len(state.Projections)
	if state.Shape == datasource.ResultScalar {
		maximumPurposes = 1 - len(state.Projections)
	}
	purposes, nextTotal, err := resolveDatabaseQueryPurposeQueue(
		ctx,
		assemblyline.DatabaseQueryPurposeAuthority{
			State: state, Collection: assemblyline.DatabaseQueryProjectionPurpose,
		},
		maximumPurposes, true, call, total,
	)
	total = nextTotal
	if err != nil {
		return state, total, err
	}
	for _, purpose := range purposes {
		leaf := assemblyline.DatabaseQueryProjectionLeafInput{State: state, Purpose: purpose}
		var calls int
		if state.Shape != datasource.ResultRecords {
			job, err := assemblyline.NewDatabaseQueryProjectionAggregateJob(leaf)
			if err != nil {
				return state, total, err
			}
			leaf.Aggregate, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_projection_aggregate", job,
				func(raw string) (datasource.AggregateOperation, error) {
					return assemblyline.DecodeDatabaseQueryProjectionAggregateLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		if leaf.Aggregate != datasource.AggregateCountRows {
			fieldID, resolved, err := assemblyline.ResolveSoleDatabaseQueryProjectionFieldLeaf(leaf)
			if err != nil {
				return state, total, err
			}
			if resolved {
				leaf.FieldID = fieldID
			} else {
				job, err := assemblyline.NewDatabaseQueryProjectionFieldJob(leaf)
				if err != nil {
					return state, total, err
				}
				leaf.FieldID, calls, err = callObjectiveDatabaseRawLeaf(
					ctx, call, "database_query_projection_field", job,
					func(raw string) (string, error) {
						return assemblyline.DecodeDatabaseQueryProjectionFieldLeaf(leaf, raw)
					},
				)
				total += calls
				if err != nil {
					return state, total, err
				}
			}
		}
		projection := datasource.RelationalProjection{
			FieldID: leaf.FieldID, Aggregate: leaf.Aggregate,
		}
		if leaf.Aggregate == "" && state.Shape == datasource.ResultTrend &&
			objectiveDatabaseTemporalField(state, leaf.FieldID) {
			job, err := assemblyline.NewDatabaseQueryProjectionTimeBucketJob(leaf)
			if err != nil {
				return state, total, err
			}
			projection.TimeBucket, calls, err = callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_projection_time_bucket", job,
				func(raw string) (datasource.TimeBucketUnit, error) {
					return assemblyline.DecodeDatabaseQueryProjectionTimeBucketLeaf(leaf, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
		}
		state.Projections = append(state.Projections, projection)
	}
	if !assemblyline.DatabaseQueryProjectionsHaveRequiredShape(state) {
		return state, total, fmt.Errorf(
			"database query projection purpose queue exhausted before satisfying the code-owned %s shape",
			state.Shape,
		)
	}
	return state, total, nil
}
