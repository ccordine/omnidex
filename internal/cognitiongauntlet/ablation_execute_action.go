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
	request cognition.ActionRequest,
	transition cognition.Transition,
	cycle int,
	budget RunBudget,
) (cognition.Transition, error) {
	if execution.Resources.EnvironmentActions >= budget.EnvironmentActions ||
		execution.Resources.ToolOperations >= budget.ToolOperations {
		id := fmt.Sprintf("ablation-action-budget-%03d", cycle)
		if err := terminateAblation(
			recorder, execution, "resource_budget", id,
			"The frozen environment-action budget was exhausted.", true,
		); err != nil {
			return cognition.Transition{}, err
		}
		return transition, nil
	}
	schema, exists := state.catalog.Schema(request.Kind)
	if !exists {
		id := fmt.Sprintf("ablation-schema-failure-%03d", cycle)
		if err := terminateAblation(recorder, execution, "model_policy", id,
			"The model selected an action absent from the world catalog.", false); err != nil {
			return cognition.Transition{}, err
		}
		return transition, nil
	}
	actionDigest, err := digestJSON(struct {
		Episode  cognition.EpisodeID     `json:"episode"`
		Cycle    int                     `json:"cycle"`
		Request  cognition.ActionRequest `json:"request"`
		Evidence []cognition.EvidenceRef `json:"evidence"`
	}{state.episode.ID, cycle, request, decision.EvidenceRefs})
	if err != nil {
		return cognition.Transition{}, err
	}
	action, err := cognition.NewRegisteredAction(
		cognition.ActionID("environment-action-"+actionDigest), state.actor,
		schema, request, decision.EvidenceRefs,
	)
	if err != nil {
		id := fmt.Sprintf("ablation-action-contract-%03d", cycle)
		if terminalErr := terminateAblation(
			recorder, execution, "model_policy", id, err.Error(), false,
		); terminalErr != nil {
			return cognition.Transition{}, terminalErr
		}
		return transition, nil
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
		if err := appendAblationTerminal(
			recorder, transition.Current, string(failure.Code), false,
		); err != nil {
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
	if err := appendTransitionObservations(recorder, next); err != nil {
		return cognition.Transition{}, err
	}
	if err := state.recordTransition(next); err != nil {
		return cognition.Transition{}, err
	}
	execution.Revision = next.Current
	if state.workingSet != nil {
		resident := int64(state.workingSet.Usage().ResidentBytes)
		if resident > execution.Resources.PeakWorkingSetBytes {
			execution.Resources.PeakWorkingSetBytes = resident
		}
	}
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
	if err := appendAblationTerminal(recorder, transition.Current, transition.PublicOutcome, true); err != nil {
		return ablationExecution{}, err
	}
	execution.Revision = transition.Current
	execution.Outcome = Outcome{
		Terminal: true, GoalSatisfied: true, PublicOutcome: transition.PublicOutcome,
	}
	execution.Planning.ObligationsCompleted = 1
	execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
	return execution, nil
}

func failAblation(
	execution ablationExecution,
	recorder *EpisodeRecorder,
	code string,
	id string,
	message string,
	startedAt time.Time,
	budget bool,
) (ablationExecution, error) {
	if err := terminateAblation(recorder, &execution, code, id, message, budget); err != nil {
		return ablationExecution{}, err
	}
	execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
	return execution, nil
}

func terminateAblation(
	recorder *EpisodeRecorder,
	execution *ablationExecution,
	code string,
	id string,
	message string,
	budget bool,
) error {
	if err := appendAblationFailure(recorder, id, execution.Revision, code, message); err != nil {
		return err
	}
	if err := appendAblationTerminal(recorder, execution.Revision, code, false); err != nil {
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
	return nil
}
