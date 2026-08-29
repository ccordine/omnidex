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
	for {
		var calls int
		if assemblyline.DatabaseQueryProjectionsHaveRequiredShape(state) {
			switch state.Shape {
			case datasource.ResultScalar, datasource.ResultExistence:
				return state, total, nil
			}
			coverageJob, err := assemblyline.NewDatabaseQueryProjectionCoverageJob(state)
			if err != nil {
				return state, total, err
			}
			coverage, calls, err := callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_projection_coverage", coverageJob,
				func(raw string) (string, error) {
					return assemblyline.DecodeDatabaseQueryProjectionCoverageLeaf(state, raw)
				},
			)
			total += calls
			if err != nil {
				return state, total, err
			}
			if coverage == assemblyline.DatabaseQueryNoUncoveredItem {
				return state, total, nil
			}
		}
		if len(state.Projections) == datasource.MaxIntentProjections {
			return state, total, fmt.Errorf(
				"database query projections do not satisfy the required shape at the code-owned %d-item bound",
				datasource.MaxIntentProjections,
			)
		}
		leaf := assemblyline.DatabaseQueryProjectionLeafInput{State: state}
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
			fields := objectiveDatabaseProjectionFields(state, leaf.Aggregate)
			if len(fields) == 0 {
				return state, total, fmt.Errorf("database query projection has no compatible field")
			}
			if len(fields) == 1 {
				leaf.FieldID = fields[0]
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
}

func objectiveDatabaseProjectionFields(
	state assemblyline.DatabaseQueryIntentLeafState,
	aggregate datasource.AggregateOperation,
) []string {
	fields := []string{}
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			compatible := true
			switch aggregate {
			case datasource.AggregateSum, datasource.AggregateAverage:
				compatible = column.TypeCategory == datasource.TypeInteger ||
					column.TypeCategory == datasource.TypeDecimal
			}
			if compatible {
				fields = append(fields, column.ID)
			}
		}
	}
	return fields
}
