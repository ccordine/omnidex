package labyrinth

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func RunExtendedOracle(
	ctx context.Context,
	generated ExtendedCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) (ExtendedOracleRun, error) {
	if err := generated.Validate(); err != nil {
		return ExtendedOracleRun{}, err
	}
	environment, err := newExtendedOracleEnvironment(generated, episode, actor)
	if err != nil {
		return ExtendedOracleRun{}, err
	}
	start, err := environment.Start(ctx, generated.execution.Ref())
	if err != nil {
		return ExtendedOracleRun{}, err
	}
	transitions := []cognition.Transition{start}
	current := start.Current
	observations := make(map[cognition.ActionID][]cognition.Observation)
	for _, witness := range generated.oracle.Witness {
		transition, err := applyExtendedWitnessAction(
			ctx, environment, episode, actor, generated, witness, current, observations,
		)
		if err != nil {
			return ExtendedOracleRun{}, fmt.Errorf("extended witness %s: %w", witness.ID, err)
		}
		transitions = append(transitions, transition)
		current = transition.Current
		observations[witness.ID] = append([]cognition.Observation{}, transition.Observations...)
	}
	terminal := transitions[len(transitions)-1].Terminal
	if !terminal {
		return ExtendedOracleRun{}, fmt.Errorf("%w: extended witness did not satisfy its goal", ErrGeneration)
	}
	return ExtendedOracleRun{Transitions: transitions, Terminal: terminal}, nil
}

func VerifyExtendedInvalidRails(
	ctx context.Context,
	generated ExtendedCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) error {
	if err := generated.Validate(); err != nil {
		return err
	}
	for index, rail := range generated.oracle.InvalidRails {
		railEpisode := cognition.EpisodeRef{ID: cognition.EpisodeID(fmt.Sprintf("%s-rail-%d", episode.ID, index+1))}
		environment, err := newExtendedOracleEnvironment(generated, railEpisode, actor)
		if err != nil {
			return err
		}
		start, err := environment.Start(ctx, generated.execution.Ref())
		if err != nil {
			return err
		}
		current := start.Current
		observations := make(map[cognition.ActionID][]cognition.Observation)
		for witnessIndex := 0; witnessIndex < rail.PrefixActions; witnessIndex++ {
			witness := generated.oracle.Witness[witnessIndex]
			transition, applyErr := applyExtendedWitnessAction(
				ctx, environment, railEpisode, actor, generated, witness, current, observations,
			)
			if applyErr != nil {
				return fmt.Errorf("prepare invalid rail %s: %w", rail.ID, applyErr)
			}
			current = transition.Current
			observations[witness.ID] = append([]cognition.Observation{}, transition.Observations...)
		}
		schema, exists := generated.execution.Catalog().Schema(rail.Request.Kind)
		if !exists {
			return fmt.Errorf("%w: invalid rail schema is absent", ErrGeneration)
		}
		refs := allExtendedEvidence(observations)
		if schema.EvidencePolicy == cognition.EvidenceForbidden {
			refs = []cognition.EvidenceRef{}
		}
		action, err := cognition.NewRegisteredAction(
			cognition.ActionID("invalid-"+rail.ID), actor, schema, rail.Request, refs,
		)
		if err != nil {
			return err
		}
		transition, applyErr := environment.Apply(ctx, railEpisode, current, action)
		switch rail.Outcome {
		case ExtendedRailRejected:
			var failure cognition.ActionFailure
			if !errors.As(applyErr, &failure) || failure.Code != rail.FailureCode {
				return fmt.Errorf("%w: invalid rail %s failure=%v", ErrGeneration, rail.ID, applyErr)
			}
		case ExtendedRailIrreversibleDeadEnd:
			if applyErr != nil || transition.Terminal {
				return fmt.Errorf("%w: irreversible rail %s did not commit a nonterminal transition", ErrGeneration, rail.ID)
			}
			reachable, reachErr := extendedGoalReachable(environment, generated.execution, rail.ID)
			if reachErr != nil {
				return reachErr
			}
			if reachable {
				return fmt.Errorf("%w: irreversible rail %s retained a path to the goal", ErrGeneration, rail.ID)
			}
		default:
			return fmt.Errorf("%w: invalid rail outcome is unregistered", ErrGeneration)
		}
	}
	return nil
}

func extendedGoalReachable(environment *Environment, scenario Scenario, railID string) (bool, error) {
	environment.mu.Lock()
	facts := environment.facts.clone()
	environment.mu.Unlock()
	definition := scenario.definition.clone()
	definition.initialFacts = facts.sorted()
	definition.sha256 = ""
	if err := definition.reseal(); err != nil {
		return false, err
	}
	counterfactual, err := NewScenario(
		cognition.ScenarioID(string(scenario.Ref().ID)+"-"+railID), definition, scenario.descriptor,
	)
	if err != nil {
		return false, err
	}
	_, err = Solve(counterfactual, SolverBounds{MaxStates: 20_000, MaxGroundActions: MaxSolverGroundActions})
	if errors.Is(err, ErrUnsolvable) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func newExtendedOracleEnvironment(
	generated ExtendedCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) (*Environment, error) {
	return NewEnvironment(generated.execution, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != actor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
}

func applyExtendedWitnessAction(
	ctx context.Context,
	environment *Environment,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	generated ExtendedCase,
	witness WitnessAction,
	current cognition.WorldRevision,
	observations map[cognition.ActionID][]cognition.Observation,
) (cognition.Transition, error) {
	schema, exists := generated.execution.Catalog().Schema(witness.Request.Kind)
	if !exists {
		return cognition.Transition{}, fmt.Errorf("extended witness schema is absent")
	}
	refs := extendedConsumerEvidence(generated.oracle, witness.ID, observations)
	action, err := cognition.NewRegisteredAction(witness.ID, actor, schema, witness.Request, refs)
	if err != nil {
		return cognition.Transition{}, err
	}
	return environment.Apply(ctx, episode, current, action)
}

func extendedConsumerEvidence(
	oracle ExtendedOracle,
	consumer cognition.ActionID,
	observations map[cognition.ActionID][]cognition.Observation,
) []cognition.EvidenceRef {
	result := make([]cognition.EvidenceRef, 0)
	for _, use := range oracle.EvidenceUses {
		if use.RequiredByActionID != consumer {
			continue
		}
		for _, observation := range observations[use.AcquisitionActionID] {
			result = append(result, observation.EvidenceRef())
		}
	}
	return uniqueExtendedEvidence(result)
}

func allExtendedEvidence(observations map[cognition.ActionID][]cognition.Observation) []cognition.EvidenceRef {
	result := make([]cognition.EvidenceRef, 0)
	for _, values := range observations {
		for _, observation := range values {
			result = append(result, observation.EvidenceRef())
		}
	}
	return uniqueExtendedEvidence(result)
}

func uniqueExtendedEvidence(values []cognition.EvidenceRef) []cognition.EvidenceRef {
	seen := make(map[cognition.EvidenceRef]struct{}, len(values))
	result := make([]cognition.EvidenceRef, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
