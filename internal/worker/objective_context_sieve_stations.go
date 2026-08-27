package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveContextSieveStations struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveContextSieveStations) Generate(
	ctx context.Context,
	input assemblyline.ContextSearchTermsInput,
) (assemblyline.ContextSearchTermsDecision, contextcompiler.StationReceipt, error) {
	job, err := assemblyline.NewContextSearchTermsJob(input)
	if err != nil {
		return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{}, err
	}
	if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
		decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.ContextSearchTermsDecision](
			ctx, adapter.runtime, "context_search_terms", job,
			station.ContextSearchTerms,
			func() (string, error) {
				return objectiveContextStationModel(
					adapter.runtime, input.Scope, station.ContextSearchTerms,
				)
			},
			func(value assemblyline.ContextSearchTermsDecision) error {
				return value.ValidateFor(input)
			},
		)
		return decision, contextcompiler.StationReceipt{
			Calls: receipt.Calls, Reused: receipt.Reused,
		}, err
	}
	modelName, err := objectiveContextStationModel(
		adapter.runtime, input.Scope, station.ContextSearchTerms,
	)
	if err != nil {
		return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ContextSearchTermsDecision](
		ctx, adapter.runtime, modelName, "context_search_terms", job,
		func(value assemblyline.ContextSearchTermsDecision) error { return value.ValidateFor(input) },
	)
	return decision, contextcompiler.StationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveContextSieveStations) SelectRelevant(
	ctx context.Context,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, contextcompiler.StationReceipt, error) {
	job, err := assemblyline.NewContextRelevanceJob(input)
	if err != nil {
		return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{}, err
	}
	if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
		decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.ContextRelevanceDecision](
			ctx, adapter.runtime, "context_relevance", job,
			station.ContextRelevance,
			func() (string, error) {
				return objectiveContextStationModel(
					adapter.runtime, input.Scope, station.ContextRelevance,
				)
			},
			func(value assemblyline.ContextRelevanceDecision) error {
				return value.ValidateFor(input)
			},
		)
		return decision, contextcompiler.StationReceipt{
			Calls: receipt.Calls, Reused: receipt.Reused,
		}, err
	}
	modelName, err := objectiveContextStationModel(
		adapter.runtime, input.Scope, station.ContextRelevance,
	)
	if err != nil {
		return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{}, err
	}
	if executor := adapter.runtime.svc.browserContextRelevance; executor != nil {
		decision, err := executor.ExecuteContextRelevance(ctx, modelName, input)
		if err != nil {
			return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{Calls: 1}, err
		}
		if err := decision.ValidateFor(input); err != nil {
			return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{Calls: 1}, err
		}
		return decision, contextcompiler.StationReceipt{Calls: 1}, nil
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ContextRelevanceDecision](
		ctx, adapter.runtime, modelName, "context_relevance", job,
		func(value assemblyline.ContextRelevanceDecision) error { return value.ValidateFor(input) },
	)
	return decision, contextcompiler.StationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveContextSieveStations) Minify(
	ctx context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, contextcompiler.StationReceipt, error) {
	job, err := assemblyline.NewContextMinificationJob(input)
	if err != nil {
		return assemblyline.ContextMinificationDecision{}, contextcompiler.StationReceipt{}, err
	}
	if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
		decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.ContextMinificationDecision](
			ctx, adapter.runtime, "context_minification", job,
			station.ContextMinification,
			func() (string, error) {
				return objectiveContextStationModel(
					adapter.runtime, input.Scope, station.ContextMinification,
				)
			},
			func(value assemblyline.ContextMinificationDecision) error {
				return value.ValidateFor(input)
			},
		)
		return decision, contextcompiler.StationReceipt{
			Calls: receipt.Calls, Reused: receipt.Reused,
		}, err
	}
	modelName, err := objectiveContextStationModel(
		adapter.runtime, input.Scope, station.ContextMinification,
	)
	if err != nil {
		return assemblyline.ContextMinificationDecision{}, contextcompiler.StationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ContextMinificationDecision](
		ctx, adapter.runtime, modelName, "context_minification", job,
		func(value assemblyline.ContextMinificationDecision) error { return value.ValidateFor(input) },
	)
	return decision, contextcompiler.StationReceipt{Calls: calls}, err
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
