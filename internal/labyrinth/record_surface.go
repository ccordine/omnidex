package labyrinth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const RecordSurfaceVersionV1 = "record-surface.v1"

// RecordEnvironment is a second benchmark rendering of the sealed symbolic
// kernel. Its public execution boundary remains cognition.Environment; record
// state and preparation authority never leave the host.
type RecordEnvironment struct {
	kernel  *Environment
	surface *recordSurface
}

var _ cognition.Environment = (*RecordEnvironment)(nil)

func NewRecordEnvironment(
	scenario Scenario,
	episode cognition.EpisodeRef,
	authorize AttemptAuthorizer,
) (*RecordEnvironment, error) {
	if err := scenario.Validate(); err != nil {
		return nil, err
	}
	if scenario.artifactCorpus != nil {
		return nil, fmt.Errorf("%w: record surface v1 does not support lazy artifact corpora", ErrSurfaceLimit)
	}
	if err := validateV1SurfaceCatalog(scenario.Catalog()); err != nil {
		return nil, err
	}
	if _, err := newRecordSurfaceState(scenario); err != nil {
		return nil, err
	}
	surface := &recordSurface{}
	kernel, err := newEnvironment(scenario, episode, authorize, surface)
	if err != nil {
		return nil, err
	}
	return &RecordEnvironment{kernel: kernel, surface: surface}, nil
}

func (environment *RecordEnvironment) Start(
	ctx context.Context,
	scenario cognition.ScenarioRef,
) (cognition.Transition, error) {
	return environment.kernel.Start(ctx, scenario)
}

func (environment *RecordEnvironment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	return environment.kernel.Apply(ctx, episode, expected, action)
}

func (environment *RecordEnvironment) EvaluateGoal(
	ctx context.Context,
	episode cognition.EpisodeRef,
	revision cognition.WorldRevision,
	desired cognition.GoalExpression,
) (bool, error) {
	if environment == nil || environment.kernel == nil {
		return false, fmt.Errorf("%w: record environment is unavailable", cognition.ErrInvalidRevision)
	}
	return environment.kernel.EvaluateGoal(ctx, episode, revision, desired)
}

func (environment *RecordEnvironment) Close() error {
	environment.kernel.mu.Lock()
	defer environment.kernel.mu.Unlock()
	return environment.surface.Close()
}

func (environment *RecordEnvironment) MarshalJSON() ([]byte, error) {
	return json.Marshal(environment.kernel)
}
