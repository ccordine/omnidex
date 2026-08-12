package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func applyAblationDecision(
	ctx context.Context,
	environment cognition.Environment,
	recorder *EpisodeRecorder,
	state *ablationState,
	execution *ablationExecution,
	decision cognition.CognitionDecision,
	callID string,
	request cognition.ActionRequest,
	transition cognition.Transition,
	cycle int,
	budget RunBudget,
) (cognition.Transition, error) {
	if execution.Resources.EnvironmentActions >= budget.EnvironmentActions ||
		execution.Resources.ToolOperations >= budget.ToolOperations {
		state.recordNoAction(
			uint32(cycle), callID, ablationAcceptedNoAction, "resource_budget",
		)
		id := fmt.Sprintf("ablation-action-budget-%03d", cycle)
		if err := terminateAblation(
			recorder, execution, ablationTerminalCause{
				Kind: ablationTerminalNoDispatch, CallOrdinal: uint32(cycle),
				Reason: "resource_budget", CompletedCalls: execution.Resources.PolicyCallsConsumed,
				CompletedCycles: cycle,
			}, "resource_budget", id,
			"The frozen environment-action budget was exhausted.", true,
		); err != nil {
			return cognition.Transition{}, err
		}
		return transition, nil
	}
	schema, exists := state.catalog.Schema(request.Kind)
	if !exists {
		return cognition.Transition{}, fmt.Errorf(
			"accepted cognition decision selected action %q absent from the authoritative catalog",
			request.Kind,
		)
	}
	action, err := newAblationRegisteredAction(
		state.episode, state.actor, schema, uint32(cycle), decision, request,
	)
	if err != nil {
		return cognition.Transition{}, fmt.Errorf(
			"accepted cognition decision could not produce its registered action: %w", err,
		)
	}
	next, applyErr := environment.Apply(ctx, state.episode, transition.Current, action)
	execution.Resources.EnvironmentActions++
	execution.Resources.ToolOperations++
	countOracleOperation(&execution.Resources, request.Kind)
	if applyErr != nil {
		var failure cognition.ActionFailure
		if !errors.As(applyErr, &failure) {
			return cognition.Transition{}, fmt.Errorf("apply cognition ablation action: %w", applyErr)
		}
		if err := appendAblationActionFailure(recorder, action, transition.Current, failure); err != nil {
			return cognition.Transition{}, err
		}
		execution.Planning.InvalidActions++
		state.appendAction(request, failure.PublicMessage, true)
		state.recordActionEvidence(uint32(cycle), callID, ActionTrace{
			Schema: ActionTraceSchemaV1, Action: action.Clone(),
			ExpectedRevision: transition.Current, Failure: &failure,
		})
		if err := setPendingAblationTerminal(
			execution, transition.Current, string(failure.Code), false, string(failure.Code),
		); err != nil {
			return cognition.Transition{}, err
		}
		if err := setAblationTerminalCause(execution, ablationTerminalCause{
			Kind: ablationTerminalActionFailure, CallOrdinal: uint32(cycle),
			ActionID: action.ID, Reason: string(failure.Code),
			CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: cycle,
		}); err != nil {
			return cognition.Transition{}, err
		}
		execution.Outcome = Outcome{
			Terminal: true, GoalSatisfied: false,
			PublicOutcome: string(failure.Code), FailureCode: string(failure.Code),
		}
		execution.FailureTrace = FailureTrace{
			PolicyRejected: true, PolicyFailureEventID: string(action.ID),
		}
		return transition, nil
	}
	if err := appendOracleAction(recorder, action, next); err != nil {
		return cognition.Transition{}, err
	}
	execution.Resources.LowLevelTransitions++
	state.appendAction(request, next.PublicOutcome, false)
	copy := next.Clone()
	state.recordActionEvidence(uint32(cycle), callID, ActionTrace{
		Schema: ActionTraceSchemaV1, Action: action.Clone(),
		ExpectedRevision: transition.Current, Transition: &copy,
	})
	if err := appendTransitionObservations(recorder, next); err != nil {
		return cognition.Transition{}, err
	}
	if err := state.recordTransition(next); err != nil {
		return cognition.Transition{}, err
	}
	execution.Revision = next.Current
	recordAblationWorkingSetPeak(state, &execution.Resources)
	return next, nil
}

func completeAblation(
	ctx context.Context,
	completion ablationCompletionAuthority,
	state *ablationState,
	execution ablationExecution,
	transition cognition.Transition,
	startedAt time.Time,
	recorder *EpisodeRecorder,
) (ablationExecution, error) {
	if !transition.Terminal || transition.PublicOutcome == "" {
		return ablationExecution{}, fmt.Errorf("ablation completion lacks terminal environment authority")
	}
	if completion == nil {
		return ablationExecution{}, fmt.Errorf("ablation completion authority is unavailable")
	}
	satisfied, err := completion.Satisfied(ctx, state, transition)
	if err != nil {
		return ablationExecution{}, fmt.Errorf("evaluate ablation goal: %w", err)
	}
	if !satisfied {
		return ablationExecution{}, fmt.Errorf("ablation environment declared terminal without satisfying the exact goal")
	}
	execution.Revision = transition.Current
	execution.Outcome = Outcome{
		Terminal: true, GoalSatisfied: true, PublicOutcome: transition.PublicOutcome,
	}
	execution.Planning.ObligationsCompleted = 1
	execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
	if err := setPendingAblationTerminal(
		&execution, transition.Current, transition.PublicOutcome, true, "",
	); err != nil {
		return ablationExecution{}, err
	}
	if err := setAblationTerminalCause(&execution, ablationTerminalCause{
		Kind: ablationTerminalWorld, CallOrdinal: uint32(execution.Resources.PolicyCallsConsumed),
		ActionID: transition.ActionID, CompletedCalls: execution.Resources.PolicyCallsConsumed,
		CompletedCycles: execution.Resources.PolicyCallsConsumed,
	}); err != nil {
		return ablationExecution{}, err
	}
	return execution, nil
}

func failAblation(
	execution ablationExecution,
	recorder *EpisodeRecorder,
	cause ablationTerminalCause,
	code string,
	id string,
	message string,
	startedAt time.Time,
	budget bool,
) (ablationExecution, error) {
	if err := terminateAblation(recorder, &execution, cause, code, id, message, budget); err != nil {
		return ablationExecution{}, err
	}
	execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
	return execution, nil
}

func terminateAblation(
	recorder *EpisodeRecorder,
	execution *ablationExecution,
	cause ablationTerminalCause,
	code string,
	id string,
	message string,
	budget bool,
) error {
	if err := appendAblationFailure(recorder, id, execution.Revision, code, message); err != nil {
		return err
	}
	execution.Outcome = Outcome{
		Terminal: true, GoalSatisfied: false, PublicOutcome: code, FailureCode: code,
	}
	if budget {
		execution.FailureTrace = FailureTrace{BudgetExhausted: true, BudgetEventID: id}
	} else {
		execution.FailureTrace = FailureTrace{PolicyRejected: true, PolicyFailureEventID: id}
	}
	if err := setPendingAblationTerminal(execution, execution.Revision, code, false, code); err != nil {
		return err
	}
	return setAblationTerminalCause(execution, cause)
}
