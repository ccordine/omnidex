package labyrinth

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/cognition"
)

type actionReceipt struct {
	requestSHA256 string
	expected      cognition.WorldRevision
	transition    *cognition.Transition
	failure       *cognition.ActionFailure
	cause         error
}

// Environment is an in-memory deterministic benchmark host. Complete facts
// remain private; callers receive only cognition transitions and observations.
type Environment struct {
	mu           sync.Mutex
	scenario     Scenario
	episode      cognition.EpisodeRef
	authorize    AttemptAuthorizer
	entities     map[EntityID]Entity
	predicates   map[cognition.PredicateName]PredicateSchema
	started      bool
	current      cognition.WorldRevision
	facts        factSet
	totalCost    int64
	terminal     bool
	receipts     map[cognition.ActionID]actionReceipt
	observations map[cognition.ObservationID]cognition.Observation
	surface      environmentSurface
	surfaceState surfacePreparation
}

var _ cognition.Environment = (*Environment)(nil)

func NewEnvironment(
	scenario Scenario,
	episode cognition.EpisodeRef,
	authorize AttemptAuthorizer,
) (*Environment, error) {
	return newEnvironment(scenario, episode, authorize, nil)
}

func newEnvironment(
	scenario Scenario,
	episode cognition.EpisodeRef,
	authorize AttemptAuthorizer,
	surface environmentSurface,
) (*Environment, error) {
	if err := scenario.Validate(); err != nil {
		return nil, err
	}
	if err := episode.Validate(); err != nil {
		return nil, err
	}
	if authorize == nil {
		return nil, fmt.Errorf("%w: attempt authorizer is required", cognition.ErrAuthorityDenied)
	}
	entities, _, err := validateEntities(scenario.definition.entities)
	if err != nil {
		return nil, err
	}
	predicates, err := validatePredicateSchemas(scenario.definition.predicateSchemas, entityKinds(entities))
	if err != nil {
		return nil, err
	}
	return &Environment{
		scenario:     scenario.clone(),
		episode:      episode,
		authorize:    authorize,
		entities:     entities,
		predicates:   predicates,
		receipts:     make(map[cognition.ActionID]actionReceipt),
		observations: make(map[cognition.ObservationID]cognition.Observation),
		surface:      surface,
	}, nil
}

func (environment *Environment) Start(
	ctx context.Context,
	requested cognition.ScenarioRef,
) (cognition.Transition, error) {
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	if err := requested.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	if requested != environment.scenario.ref {
		return cognition.Transition{}, fmt.Errorf("%w: requested identity does not match the sealed scenario", cognition.ErrInvalidScenario)
	}
	if environment.started {
		return cognition.Transition{}, ErrAlreadyStarted
	}
	facts := newFactSet(environment.scenario.definition.initialFacts)
	terminal := goalSatisfied(environment.scenario.definition.goal, facts)
	preparation := surfacePreparation{}
	if environment.surface != nil {
		var err error
		preparation, err = environment.surface.Start(ctx, environment.scenario)
		if err != nil {
			return cognition.Transition{}, err
		}
		if err := preparation.Validate(); err != nil {
			return cognition.Transition{}, err
		}
		if preparation.Operation != "observe" {
			return cognition.Transition{}, fmt.Errorf("%w: surface start must produce observe", ErrSurfaceOperation)
		}
	}
	revision, err := revisionFor(
		environment.scenario.ref, environment.scenario.definitionSHA256, environment.episode,
		1, "", "", "", preparation.StateSHA256, facts, 0, terminal,
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	var observation cognition.Observation
	if environment.surface == nil {
		observation, err = buildObservation(
			"", revision, facts, terminal, environment.entities, environment.predicates,
			environment.scenario.descriptor.Records, environment.scenario.artifactCorpus, nil,
		)
	} else {
		observation, err = buildSurfaceObservation(
			"", revision, facts, terminal, environment.entities, environment.predicates, preparation,
		)
	}
	if err != nil {
		return cognition.Transition{}, err
	}
	transition := cognition.Transition{
		Current: revision, Observations: []cognition.Observation{observation},
		Effects: []cognition.Effect{}, Terminal: terminal,
	}
	if terminal {
		transition.PublicOutcome = PublicOutcomeGoalSatisfied
	}
	if err := transition.ValidateStart(); err != nil {
		return cognition.Transition{}, err
	}
	environment.started = true
	environment.current = revision
	environment.facts = facts
	environment.terminal = terminal
	environment.surfaceState = preparation.clone()
	environment.observations[observation.ID] = observation
	return transition.Clone(), nil
}

func (environment *Environment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	if err := action.Actor.Validate(); err != nil {
		return cognition.Transition{}, fmt.Errorf("%w: %v", cognition.ErrInvalidAction, err)
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	if err := environment.authorize(ctx, action.Actor); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return cognition.Transition{}, contextErr
		}
		return cognition.Transition{}, environment.failure(
			cognition.ActionFailureUnauthorized, action, expected,
			"The current execution attempt is not authorized.", cognition.ErrAuthorityDenied,
		)
	}
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	return environment.applyLocked(ctx, episode, expected, action)
}

func (environment *Environment) applyLocked(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	if !environment.started {
		return cognition.Transition{}, ErrNotStarted
	}
	if err := episode.Validate(); err != nil || episode != environment.episode {
		return cognition.Transition{}, environment.failure(
			cognition.ActionFailureStaleRevision, action, expected,
			"The requested episode is not current.", cognition.ErrInvalidRevision,
		)
	}
	if err := expected.Validate(); err != nil {
		return cognition.Transition{}, err
	}
	requestSHA256, err := canonicalActionSHA256(action)
	if err != nil {
		return cognition.Transition{}, err
	}
	if receipt, exists := environment.receipts[action.ID]; exists {
		if receipt.requestSHA256 != requestSHA256 || receipt.expected != expected {
			return cognition.Transition{}, environment.failure(
				cognition.ActionFailureIdempotencyConflict, action, expected,
				"The action identity is already bound to a different request.", ErrReplayConflict,
			)
		}
		return receipt.result()
	}
	definition, exists := environment.scenario.action(action.Request.Kind)
	if !exists || action.Validate(definition.Schema) != nil {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureInvalidAction, action, expected, requestSHA256,
			"The registered action does not match the public action catalog.", cognition.ErrInvalidAction,
		)
	}
	if expected != environment.current {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureStaleRevision, action, expected, requestSHA256,
			"The expected world revision is not current.", cognition.ErrInvalidRevision,
		)
	}
	if environment.terminal {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureTerminal, action, expected, requestSHA256,
			"The environment has already reached its terminal predicate.", ErrTerminal,
		)
	}
	if len(environment.receipts) >= MaxEpisodeTransitions {
		return cognition.Transition{}, environment.failure(
			cognition.ActionFailureBudget, action, expected,
			"The episode transition budget is exhausted.", ErrTransitionLimit,
		)
	}
	if err := validateActionEvidence(action, environment.observations); err != nil {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureInvalidAction, action, expected, requestSHA256,
			"The action cites evidence that this environment did not produce.", cognition.ErrInvalidEvidence,
		)
	}
	if err := validateCausalActionEvidence(
		action, environment.facts, environment.observations, environment.scenario.descriptor.Records,
	); err != nil {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureInvalidAction, action, expected, requestSHA256,
			"The action is not grounded by the exact acquired evidence.", cognition.ErrInvalidEvidence,
		)
	}
	return environment.commitCandidate(ctx, definition, expected, action, requestSHA256)
}
