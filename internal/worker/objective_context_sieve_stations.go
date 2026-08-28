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

func (adapter portableObjectiveContextSieveStations) Generate(
	ctx context.Context,
	input assemblyline.ContextSearchTermsInput,
) (assemblyline.ContextSearchTermsDecision, contextcompiler.StationReceipt, error) {
	if adapter.runtime == nil {
		return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{}, fmt.Errorf(
			"context search terms require objective runtime authority",
		)
	}
	resolveModel := func() (string, error) {
		return objectiveContextStationModel(
			adapter.runtime, input.Scope, station.ContextSearchTerms,
		)
	}
	modelName := ""
	if input.Scope != assemblyline.ContextScopeRoleplaySimulation {
		var err error
		modelName, err = resolveModel()
		if err != nil {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{}, err
		}
	}
	call := func(
		subject string,
		job assemblyline.PortableJob,
		decode objectiveRawLeafDecoder[string],
	) (string, objectiveStationReceipt, error) {
		if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
			return runObjectiveReusablePortableRawLeafCall(
				ctx, adapter.runtime, subject, job, station.ContextSearchTerms,
				resolveModel, decode, func(string) error { return nil },
			)
		}
		value, calls, err := runObjectivePortableRawLeafCall(
			ctx, adapter.runtime, modelName, subject, job,
			decode, func(string) error { return nil },
		)
		return value, objectiveStationReceipt{Calls: calls}, err
	}

	terms := make([]string, 0, assemblyline.MaxContextSearchTerms)
	totalCalls, allReused := 0, true
	for {
		leafInput := assemblyline.ContextSearchTermLeafInput{
			ExactInstruction: input.ExactInstruction, Scope: input.Scope,
			AcceptedTerms: append([]string{}, terms...),
		}
		coverageJob, err := assemblyline.NewContextSearchTermCoverageJob(leafInput)
		if err != nil {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{}, err
		}
		coverage, receipt, err := call(
			"context_search_term_coverage", coverageJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeContextSearchTermCoverageLeaf(leafInput, raw)
			},
		)
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, err
		}
		if coverage == assemblyline.ContextNoUncoveredTerm {
			decision, err := assemblyline.AssembleContextSearchTermsDecision(input, terms)
			return decision, contextcompiler.StationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, err
		}
		if len(terms) == assemblyline.MaxContextSearchTerms {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, fmt.Errorf(
				"context search term coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxContextSearchTerms,
			)
		}
		termJob, err := assemblyline.NewContextSearchTermJob(leafInput)
		if err != nil {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, err
		}
		term, receipt, err := call(
			"context_search_term", termJob,
			func(raw string) (string, error) {
				return assemblyline.DecodeContextSearchTermLeaf(leafInput, raw)
			},
		)
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return assemblyline.ContextSearchTermsDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, err
		}
		terms = append(terms, term)
	}
}

func (adapter portableObjectiveContextSieveStations) SelectRelevant(
	ctx context.Context,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, contextcompiler.StationReceipt, error) {
	if adapter.runtime == nil || adapter.runtime.svc == nil {
		return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{}, fmt.Errorf(
			"context relevance requires objective runtime authority",
		)
	}
	resolveModel := func() (string, error) {
		return objectiveContextStationModel(
			adapter.runtime, input.Scope, station.ContextRelevance,
		)
	}
	modelName := ""
	if input.Scope != assemblyline.ContextScopeRoleplaySimulation {
		var err error
		modelName, err = resolveModel()
		if err != nil {
			return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{}, err
		}
	}

	selected := make([]string, 0, input.MaxSelections)
	totalCalls, allReused := 0, true
	for len(selected) < input.MaxSelections {
		leafInput := assemblyline.ContextRelevanceSelectionInput{
			Authority:            input,
			AcceptedCandidateIDs: append([]string{}, selected...),
		}
		job, err := assemblyline.NewContextRelevanceSelectionJob(leafInput)
		if err != nil {
			return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, err
		}
		var decision assemblyline.ContextRelevanceSelectionDecision
		if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
			var receipt objectiveStationReceipt
			decision, receipt, err = runObjectiveReusablePortableRawLeafCall(
				ctx, adapter.runtime, "context_relevance_selection", job,
				station.ContextRelevance, resolveModel,
				func(raw string) (assemblyline.ContextRelevanceSelectionDecision, error) {
					return assemblyline.DecodeContextRelevanceSelectionDecision(leafInput, raw)
				},
				func(value assemblyline.ContextRelevanceSelectionDecision) error {
					return value.ValidateFor(leafInput)
				},
			)
			totalCalls += receipt.Calls
			allReused = allReused && receipt.Reused
		} else if executor := adapter.runtime.svc.browserContextRelevance; executor != nil {
			decision, err = executor.ExecuteContextRelevance(ctx, modelName, leafInput)
			totalCalls++
			allReused = false
			if err == nil {
				err = decision.ValidateFor(leafInput)
			}
		} else {
			var calls int
			decision, calls, err = runObjectivePortableRawLeafCall(
				ctx, adapter.runtime, modelName, "context_relevance_selection", job,
				func(raw string) (assemblyline.ContextRelevanceSelectionDecision, error) {
					return assemblyline.DecodeContextRelevanceSelectionDecision(leafInput, raw)
				},
				func(value assemblyline.ContextRelevanceSelectionDecision) error {
					return value.ValidateFor(leafInput)
				},
			)
			totalCalls += calls
			allReused = false
		}
		if err != nil {
			return assemblyline.ContextRelevanceDecision{}, contextcompiler.StationReceipt{Calls: totalCalls}, err
		}
		if decision.CandidateID == assemblyline.ContextRelevanceNoCandidate {
			break
		}
		selected = append(selected, decision.CandidateID)
	}
	assembled, err := assemblyline.AssembleContextRelevanceDecision(input, selected)
	return assembled, contextcompiler.StationReceipt{
		Calls: totalCalls, Reused: allReused,
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
	if input.Scope == assemblyline.ContextScopeRoleplaySimulation {
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
	decision, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, modelName, "context_minification", job,
		func(raw string) (assemblyline.ContextMinificationDecision, error) {
			return assemblyline.DecodeContextMinificationDecision(input, raw)
		},
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
