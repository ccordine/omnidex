package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type Machine struct {
	objective Objective
	config    Config
	relevance RelevanceStation
	synthesis GroundedSynthesisStation
	contracts acquisitionContracts
}

func New(
	objective Objective,
	config Config,
	acquisition Acquisition,
	relevance RelevanceStation,
	synthesis GroundedSynthesisStation,
) (*Machine, error) {
	if err := validateObjective(objective); err != nil {
		return nil, err
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if nilInterface(acquisition) || nilInterface(relevance) || nilInterface(synthesis) {
		return nil, fmt.Errorf("%w: acquisition, relevance, and synthesis are required", ErrInvalidConfiguration)
	}
	limits := acquisition.Limits()
	if limits.MaxDocuments < 1 || limits.MaxDocuments > 32 ||
		config.MaxFetchCandidates > limits.MaxDocuments {
		return nil, fmt.Errorf(
			"%w: workflow fetch bound %d exceeds deterministic acquisition bound %d",
			ErrInvalidConfiguration, config.MaxFetchCandidates, limits.MaxDocuments,
		)
	}
	contracts, err := newAcquisitionContracts(acquisition, config.MaxFetchCandidates)
	if err != nil {
		return nil, fmt.Errorf("%w: acquisition contracts: %w", ErrInvalidConfiguration, err)
	}
	return &Machine{
		objective: objective, config: config,
		relevance: relevance, synthesis: synthesis, contracts: contracts,
	}, nil
}

func (machine *Machine) Run(ctx context.Context) (Result, error) {
	if ctx == nil {
		return Result{}, ErrNilContext
	}
	if machine == nil {
		return Result{}, fmt.Errorf("%w: machine is nil", ErrInvalidConfiguration)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	const attemptLimit uint16 = 2
	result := Result{Objective: cloneObjective(machine.objective), AcquisitionAttemptLimit: int(attemptLimit)}
	fail := func(runErr error) (Result, error) {
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return Result{}, runErr
		}
		cloned := cloneResult(result)
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		return cloned, runErr
	}
	if err := machine.gatherRelevantEvidence(ctx, &result); err != nil {
		return fail(err)
	}
	evidence := result.Evidence
	projected := result.Projected
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	decision, err := machine.synthesis.Synthesize(ctx, GroundedSynthesisCall{
		Question:          machine.objective.Question,
		Context:           assemblyline.CloneObjectiveContext(machine.objective.Context),
		Evidence:          cloneProjection(projected),
		MaxParagraphs:     machine.config.MaxSynthesisParagraphs,
		MaxParagraphBytes: machine.config.MaxSynthesisParagraphBytes,
	})
	if err != nil {
		return fail(fmt.Errorf("grounded synthesis station: %w", err))
	}
	maximumSynthesisCalls := (1 + machine.config.MaxSynthesisParagraphs*(len(projected)+1)) *
		exactPortableSemanticLeafCalls
	receipt, err := decision.CallLedger.ValidateForMaximum(
		"web grounded synthesis decision", maximumSynthesisCalls,
	)
	if err != nil {
		return fail(fmt.Errorf("%w: %v", ErrInvalidSynthesis, err))
	}
	if decision.SemanticCalls != receipt.Calls {
		return fail(fmt.Errorf(
			"%w: grounded synthesis reported %d calls but its exact ledger proves %d",
			ErrInvalidSynthesis, decision.SemanticCalls, receipt.Calls,
		))
	}
	if err := result.CallLedger.Merge("synthesis", decision.CallLedger); err != nil {
		return fail(fmt.Errorf("%w: %v", ErrInvalidSynthesis, err))
	}
	result.SynthesisCalls++
	result.SemanticCalls += receipt.Calls
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	artifact, err := buildArtifact(decision, projected, evidence, machine.config)
	if err != nil {
		return fail(err)
	}
	result.Steps = append(result.Steps, StepSynthesisResolved)
	if err := ValidateCompletionArtifact(artifact, evidence); err != nil {
		return fail(err)
	}
	if err := commitCompletion(ctx, &result, artifact); err != nil {
		return fail(err)
	}
	return cloneResult(result), nil
}
