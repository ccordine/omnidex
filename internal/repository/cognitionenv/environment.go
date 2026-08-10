package cognitionenv

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

type Environment struct {
	investigation Investigation
	episode       cognition.EpisodeRef
	builder       EvidenceBuilder
	authorize     AttemptAuthorizer
	journal       cognition.EnvironmentJournal
}

var (
	_ cognition.Environment                = (*Environment)(nil)
	_ cognitionruntime.CompletionEvaluator = (*Environment)(nil)
)

func NewEnvironment(
	investigation Investigation,
	episode cognition.EpisodeRef,
	builder EvidenceBuilder,
	authorize AttemptAuthorizer,
	journal cognition.EnvironmentJournal,
) (*Environment, error) {
	if err := validateInvestigation(investigation); err != nil {
		return nil, err
	}
	if err := episode.Validate(); err != nil {
		return nil, err
	}
	if builder == nil {
		return nil, fmt.Errorf("repository cognition evidence builder is required")
	}
	if authorize == nil {
		return nil, fmt.Errorf("%w: repository cognition authorizer is required", cognition.ErrAuthorityDenied)
	}
	if journal == nil {
		return nil, fmt.Errorf("repository cognition durable environment journal is required")
	}
	return &Environment{
		investigation: investigation, episode: episode, builder: builder, authorize: authorize,
		journal: journal,
	}, nil
}

func (environment *Environment) Start(
	ctx context.Context,
	scenario cognition.ScenarioRef,
) (cognition.Transition, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.Transition{}, err
	}
	if scenario != environment.investigation.ref {
		return cognition.Transition{}, fmt.Errorf("repository cognition rejected another scenario")
	}
	start, err := environment.startTransition()
	if err != nil {
		return cognition.Transition{}, err
	}
	return environment.journal.StartEnvironment(ctx, environment.episode, scenario, start)
}

func (environment *Environment) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.Transition{}, err
	}
	if episode != environment.episode {
		return cognition.Transition{}, environment.failure(
			action, expected, cognition.ActionFailureInvalidAction, "action belongs to another episode",
		)
	}
	if err := environment.authorize(ctx, action.Actor); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return cognition.Transition{}, contextErr
		}
		return cognition.Transition{}, environment.failure(
			action, expected, cognition.ActionFailureUnauthorized, "action actor is stale",
		)
	}
	schema, registered := environment.investigation.catalog.Schema(action.Request.Kind)
	if !registered || action.Validate(schema) != nil {
		return cognition.Transition{}, environment.failure(
			action, expected, cognition.ActionFailureInvalidAction, "action is not registered for this investigation",
		)
	}
	if receipt, replay, err := environment.journal.ReviewEnvironmentAction(
		ctx, episode, environment.investigation.ref, expected, action,
	); err != nil {
		return cognition.Transition{}, environment.journalActionError(action, expected, err)
	} else if replay {
		return environment.resultFromReceipt(receipt)
	}
	journal, err := environment.journal.EnvironmentState(
		ctx, episode, environment.investigation.ref,
	)
	if err != nil {
		return cognition.Transition{}, fmt.Errorf("repository cognition journal state: %w", err)
	}
	if err := journal.Validate(); err != nil {
		return cognition.Transition{}, fmt.Errorf("repository cognition journal state: %w", err)
	}
	state, err := environment.stateFromJournal(journal)
	if err != nil {
		return cognition.Transition{}, err
	}
	requiredEvidence, err := latestStateEvidence(journal)
	if err != nil {
		return cognition.Transition{}, err
	}
	if !containsEvidence(action.EvidenceRefs, requiredEvidence) {
		return environment.commitFailure(
			ctx, action, expected, cognition.ActionFailurePreconditionFailed,
			"action omitted the exact current investigation evidence",
		)
	}
	request, err := environment.retrievalRequest(state, action)
	if err != nil {
		return environment.commitFailure(
			ctx, action, expected, cognition.ActionFailurePreconditionFailed, err.Error(),
		)
	}
	pack, err := environment.buildEvidence(ctx, request)
	if errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) {
		return environment.commitFailure(
			ctx, action, expected, cognition.ActionFailurePreconditionFailed,
			"registered repository evidence was not found",
		)
	}
	if err != nil {
		return cognition.Transition{}, fmt.Errorf("acquire registered repository evidence: %w", err)
	}
	terminal := action.Request.Kind == environment.terminalAction()
	nextState, err := nextInvestigationState(state, action, pack, terminal)
	if err != nil {
		return cognition.Transition{}, err
	}
	if err := nextState.Validate(environment.investigation); err != nil {
		return cognition.Transition{}, err
	}
	transition, err := environment.actionTransition(expected, action, pack, nextState, terminal)
	if err != nil {
		return cognition.Transition{}, err
	}
	receipt, err := cognition.NewEnvironmentTransitionReceipt(
		environment.episode, action, expected, transition,
	)
	if err != nil {
		return cognition.Transition{}, err
	}
	receipt, err = environment.journal.CommitEnvironmentAction(
		ctx, episode, environment.investigation.ref, receipt,
	)
	if err != nil {
		return cognition.Transition{}, environment.journalActionError(action, expected, err)
	}
	return environment.resultFromReceipt(receipt)
}

func (environment *Environment) validate(ctx context.Context) error {
	if ctx == nil || environment == nil || environment.builder == nil ||
		environment.authorize == nil || environment.journal == nil {
		return fmt.Errorf("repository cognition environment is uninitialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := environment.episode.Validate(); err != nil {
		return err
	}
	return validateInvestigation(environment.investigation)
}

func validateInvestigation(value Investigation) error {
	if value.projectID < 1 || value.ref.Validate() != nil || value.goal.Validate() != nil ||
		value.catalog.Validate() != nil || value.completion.Validate() != nil {
		return fmt.Errorf("repository cognition investigation is invalid")
	}
	if value.snapshot.Validate() != nil || value.analysis.Validate(value.snapshot) != nil ||
		!value.analysis.Complete {
		return fmt.Errorf("repository cognition investigation has stale snapshot authority")
	}
	check, err := value.completion.Resolve(value.goal)
	if err != nil || check != value.completion.Check {
		return fmt.Errorf("repository cognition completion authority is invalid")
	}
	want, err := NewInvestigation(
		value.projectID, value.snapshot, value.analysis, value.need, value.operation, value.query,
	)
	if err != nil || !reflect.DeepEqual(value, want) {
		return fmt.Errorf("repository cognition investigation identity does not match its exact authority")
	}
	return nil
}

func containsEvidence(values []cognition.EvidenceRef, target cognition.EvidenceRef) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
