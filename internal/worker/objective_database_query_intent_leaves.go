package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

const maxObjectiveDatabaseQueryIntentCalls = 2048

func resolveObjectiveDatabaseQueryIntent(
	ctx context.Context,
	input assemblyline.DatabaseQueryIntentInput,
	call objectiveDatabaseRawLeafCall,
) (assemblyline.DatabaseQueryIntentDecision, int, error) {
	originalCall := call
	if originalCall == nil {
		return assemblyline.DatabaseQueryIntentDecision{}, 0, fmt.Errorf(
			"database query intent raw semantic leaf call is unavailable",
		)
	}
	boundedCalls := 0
	call = func(
		ctx context.Context,
		subject string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		if boundedCalls > maxObjectiveDatabaseQueryIntentCalls-maxTypedWorkerAttempts {
			return nil, 0, fmt.Errorf(
				"database query intent exceeded its %d-call semantic leaf bound",
				maxObjectiveDatabaseQueryIntentCalls,
			)
		}
		value, calls, err := originalCall(ctx, subject, job, decode)
		if calls < 0 || calls > maxTypedWorkerAttempts {
			return nil, calls, fmt.Errorf(
				"database query intent leaf %s reported %d calls outside 0..%d",
				subject, calls, maxTypedWorkerAttempts,
			)
		}
		boundedCalls += calls
		return value, calls, err
	}
	state := assemblyline.NewDatabaseQueryIntentLeafState(input)
	totalCalls := 0
	if len(input.SchemaProjection.Relations) == 1 {
		state.FromRelationID = input.SchemaProjection.Relations[0].ID
	} else {
		job, err := assemblyline.NewDatabaseQueryFromRelationJob(state)
		if err != nil {
			return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
		}
		value, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_from_relation", job,
			func(raw string) (string, error) {
				return assemblyline.DecodeDatabaseQueryFromRelationLeaf(state, raw)
			},
		)
		totalCalls += calls
		if err != nil {
			return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
		}
		state.FromRelationID = value
	}
	shapeJob, err := assemblyline.NewDatabaseQueryShapeJob(state)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
	}
	state.Shape, totalCalls, err = callDatabaseShapeLeaf(ctx, call, shapeJob, state, totalCalls)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
	}
	if state.Shape != datasource.ResultExistence {
		state, totalCalls, err = resolveDatabaseQueryProjections(ctx, state, call, totalCalls)
		if err != nil {
			return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
		}
	}
	state.Filters, totalCalls, err = resolveDatabaseQueryFilters(
		ctx, state, "", state.Filters, call, totalCalls,
	)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
	}
	state, totalCalls, err = resolveDatabaseQueryWindows(ctx, state, call, totalCalls)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
	}
	state, totalCalls, err = resolveDatabaseQueryExistence(ctx, state, call, totalCalls)
	if err != nil {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
	}
	if state.Shape != datasource.ResultExistence {
		state, totalCalls, err = resolveDatabaseQueryHaving(ctx, state, call, totalCalls)
		if err != nil {
			return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
		}
		state, totalCalls, err = resolveDatabaseQueryOrder(ctx, state, call, totalCalls)
		if err != nil {
			return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, err
		}
	}
	if totalCalls != boundedCalls || totalCalls > maxObjectiveDatabaseQueryIntentCalls {
		return assemblyline.DatabaseQueryIntentDecision{}, totalCalls, fmt.Errorf(
			"database query intent call accounting %d/%d violates its %d-call semantic leaf bound",
			totalCalls, boundedCalls, maxObjectiveDatabaseQueryIntentCalls,
		)
	}
	groupBy := []int{}
	switch state.Shape {
	case datasource.ResultRanking, datasource.ResultDistribution,
		datasource.ResultComparison, datasource.ResultTrend:
		for index, projection := range state.Projections {
			if projection.Aggregate == "" {
				groupBy = append(groupBy, index)
			}
		}
	}
	returnDecision := assemblyline.DatabaseQueryIntentDecision{
		FromRelationID: state.FromRelationID, Shape: state.Shape,
		Projections: state.Projections, Filters: state.Filters,
		TemporalWindows: state.TemporalWindows, Exists: state.Exists,
		GroupBy: groupBy, Having: state.Having, OrderBy: state.OrderBy,
		Limit: input.MaxRows,
	}
	decision, err := assemblyline.AssembleDatabaseQueryIntentDecision(input, returnDecision)
	return decision, totalCalls, err
}

func callDatabaseShapeLeaf(
	ctx context.Context,
	call objectiveDatabaseRawLeafCall,
	job assemblyline.PortableJob,
	state assemblyline.DatabaseQueryIntentLeafState,
	total int,
) (datasource.ResultShape, int, error) {
	value, calls, err := callObjectiveDatabaseRawLeaf(
		ctx, call, "database_query_shape", job,
		func(raw string) (datasource.ResultShape, error) {
			return assemblyline.DecodeDatabaseQueryShapeLeaf(state, raw)
		},
	)
	return value, total + calls, err
}
