package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func RunPublicAblation(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicAblationRunRequest,
) (PublicAblationRunResult, error) {
	if ctx == nil {
		return PublicAblationRunResult{}, fmt.Errorf("public cognition ablation context is nil")
	}
	if err := request.validate(bundle); err != nil {
		return PublicAblationRunResult{}, err
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	brain, err := productionBrain(
		bundle.Authority.RatGeneration, bundle.Authority.Budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	attested, err := cognitionpolicy.AttestBrain(ctx, request.Client, brain)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil || attested != frozen {
		return PublicAblationRunResult{}, fmt.Errorf("live provider or host differs from frozen ablation authority")
	}
	state, err := newAblationStateWithAuthority(
		bundle.Authority.Variant, episode, request.Actor, bundle.Goal,
		bundle.Completion, bundle.Catalog, bundle.Authority.Budget.WorkingSetBytes,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	journal := newAblationCallJournal()
	loader := &ablationProjectionLoader{}
	policy, err := cognitionpolicy.New(request.Client, attested, loader, journal)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	template, err := newAblationEpisodeTemplate(
		bundle.Authority, episode, request.OmnidexCommit, request.LedgerSchemaVersion,
		request.WorkingSetPolicyVersion, request.ProjectionPolicyVersion,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	startedAt := time.Now()
	transition, err := request.Environment.Start(ctx, bundle.Authority.Scenario)
	if err != nil {
		return PublicAblationRunResult{}, fmt.Errorf("start public cognition ablation: %w", err)
	}
	if err := appendTransitionObservations(recorder, transition); err != nil {
		return PublicAblationRunResult{}, err
	}
	if err := state.recordTransition(transition); err != nil {
		return PublicAblationRunResult{}, err
	}
	oracle := labyrinth.Oracle{}
	if request.ContaminatedOracle != nil {
		oracle = *request.ContaminatedOracle
	}
	execution, err := executeAblation(
		ctx, bundle.Authority.Budget,
		request.Environment, runtimeAblationCompletion{evaluator: request.Completion},
		recorder, state, loader, journal, policy, oracle, transition, startedAt,
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	sealed, err := recorder.Seal(
		request.EpisodeSealPath, execution.Revision, execution.Outcome,
		execution.Resources, execution.Memory, execution.Planning, RecoveryMetrics{},
	)
	if err != nil {
		return PublicAblationRunResult{}, err
	}
	result := PublicAblationRunResult{Authority: bundle.Authority, Episode: sealed}
	return result, result.Validate()
}
