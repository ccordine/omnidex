package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveContextSieveStations struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveContextSieveStations) Relate(
	ctx context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, contextcompiler.StationReceipt, error) {
	if adapter.runtime == nil || adapter.runtime.svc == nil {
		return assemblyline.ContextRelevanceRelationResult{}, contextcompiler.StationReceipt{}, fmt.Errorf(
			"context relevance requires objective runtime authority",
		)
	}
	job, err := assemblyline.NewContextRelevanceRelationJob(input)
	if err != nil {
		return assemblyline.ContextRelevanceRelationResult{}, contextcompiler.StationReceipt{}, err
	}
	resolveModel := func() (string, error) {
		return objectiveContextStationModel(
			adapter.runtime, input.Scope, station.ContextRelevance,
		)
	}
	decision, receipt, err := runObjectiveReusablePortableRawLeafCall(
		ctx, adapter.runtime, "context_relevance_relation", job,
		station.ContextRelevance, resolveModel,
		func(raw string) (assemblyline.ContextRelevanceRelationResult, error) {
			return assemblyline.DecodeContextRelevanceRelationResult(input, raw)
		},
		func(value assemblyline.ContextRelevanceRelationResult) error {
			return value.ValidateFor(input)
		},
	)
	return decision, contextcompiler.StationReceipt{
		Calls: receipt.Calls, Reused: receipt.Reused,
	}, err
}

func (adapter portableObjectiveContextSieveStations) Minify(
	ctx context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, contextcompiler.StationReceipt, error) {
	job, err := assemblyline.NewContextMinificationJob(input)
	if err != nil {
		return assemblyline.ContextMinificationDecision{}, contextcompiler.StationReceipt{}, err
	}
	decision, receipt, err := runObjectiveReusablePortableRawLeafCall(
		ctx, adapter.runtime, "context_minification", job,
		station.ContextMinification,
		func() (string, error) {
			return objectiveContextStationModel(
				adapter.runtime, input.Scope, station.ContextMinification,
			)
		},
		func(raw string) (assemblyline.ContextMinificationDecision, error) {
			return assemblyline.DecodeContextMinificationDecision(input, raw)
		},
		func(value assemblyline.ContextMinificationDecision) error { return value.ValidateFor(input) },
	)
	return decision, contextcompiler.StationReceipt{
		Calls: receipt.Calls, Reused: receipt.Reused,
	}, err
}

func objectiveContextStationModel(
	runtime *nativeRuntimeV3,
	scope assemblyline.ContextScope,
	id station.ID,
) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if scope == assemblyline.ContextScopeRoleplaySimulation {
		return objectiveRoleplaySemanticModel(runtime)
	}
	return objectiveStationModel(runtime, id)
}
