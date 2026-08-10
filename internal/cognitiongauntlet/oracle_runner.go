package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func RunOracleBaseline(
	ctx context.Context,
	fixture MicrogauntletCase,
	request OracleRunRequest,
) (result OracleBaselineResult, runErr error) {
	if ctx == nil {
		return OracleBaselineResult{}, fmt.Errorf("oracle baseline context is nil")
	}
	if err := request.Validate(); err != nil {
		return OracleBaselineResult{}, err
	}
	authority, err := fixture.PairedAuthority(
		request.Surface, request.RatGeneration, request.Repetition, request.RuntimeFingerprint,
	)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	episode, err := oracleEpisodeRef(authority)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	environment, closeEnvironment, err := newBenchmarkEnvironment(fixture, episode, request.Actor, request.Surface)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	defer func() {
		if closeErr := closeEnvironment(); closeErr != nil && runErr == nil {
			result = OracleBaselineResult{}
			runErr = fmt.Errorf("close oracle baseline environment: %w", closeErr)
		}
	}()

	template, err := oracleEpisodeTemplate(fixture, episode, request, authority)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	startedAt := time.Now()
	transition, err := environment.Start(ctx, authority.Scenario)
	if err != nil {
		return OracleBaselineResult{}, fmt.Errorf("start oracle baseline: %w", err)
	}
	if err := appendTransitionObservations(recorder, transition); err != nil {
		return OracleBaselineResult{}, err
	}
	observations := append([]cognition.Observation(nil), transition.Observations...)

	oracle := fixture.generated.PrivateOracle()
	resources := Resources{}
	for index, expected := range oracle.Witness {
		if index >= authority.Budget.EnvironmentActions || index >= authority.Budget.ToolOperations {
			return OracleBaselineResult{}, fmt.Errorf("oracle baseline exhausted its frozen action budget")
		}
		schema, exists := fixture.generated.ExecutionScenario().Catalog().Schema(expected.Request.Kind)
		if !exists || schema.Ref() != expected.Schema {
			return OracleBaselineResult{}, fmt.Errorf("oracle witness action %d is absent from the frozen catalog", index)
		}
		evidence, evidenceErr := oracleActionEvidence(schema, observations)
		if evidenceErr != nil {
			return OracleBaselineResult{}, fmt.Errorf("bind oracle action %d evidence: %w", index, evidenceErr)
		}
		action, err := cognition.NewRegisteredAction(
			expected.ID, request.Actor, schema, expected.Request, evidence,
		)
		if err != nil {
			return OracleBaselineResult{}, fmt.Errorf("register oracle action %d: %w", index, err)
		}
		transition, err = environment.Apply(ctx, episode, transition.Current, action)
		if err != nil {
			return OracleBaselineResult{}, fmt.Errorf("apply oracle action %d: %w", index, err)
		}
		if transition.Cost != expected.Cost {
			return OracleBaselineResult{}, fmt.Errorf("oracle action %d returned a different cost", index)
		}
		resources.EnvironmentActions++
		resources.LowLevelTransitions++
		resources.ToolOperations++
		countOracleOperation(&resources, expected.Request.Kind)
		if err := appendOracleAction(recorder, action, transition); err != nil {
			return OracleBaselineResult{}, err
		}
		if err := appendTransitionObservations(recorder, transition); err != nil {
			return OracleBaselineResult{}, err
		}
		observations = append(observations, transition.Observations...)
	}
	if !transition.Terminal {
		return OracleBaselineResult{}, fmt.Errorf("oracle baseline witness ended before terminal state")
	}
	if err := appendOracleTerminal(recorder, transition); err != nil {
		return OracleBaselineResult{}, err
	}
	resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
	sealed, err := recorder.Seal(
		request.EpisodeSealPath, transition.Current,
		Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: transition.PublicOutcome},
		resources, MemoryMetrics{}, PlanningMetrics{}, RecoveryMetrics{},
	)
	if err != nil {
		return OracleBaselineResult{}, err
	}
	return finishOracleBaseline(request, authority, fixture, sealed)
}

func oracleActionEvidence(
	schema cognition.ActionSchema,
	observations []cognition.Observation,
) ([]cognition.EvidenceRef, error) {
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		return nil, nil
	}
	if len(observations) == 0 || len(observations) > cognition.MaxEvidenceRefs {
		return nil, fmt.Errorf("required oracle evidence exceeds the registered action boundary")
	}
	refs := make([]cognition.EvidenceRef, len(observations))
	for index, observation := range observations {
		refs[index] = observation.EvidenceRef()
	}
	return refs, nil
}

func countOracleOperation(resources *Resources, kind cognition.ActionKind) {
	switch kind {
	case "search":
		resources.SearchOperations++
	case "read":
		resources.ReadOperations++
	}
}
