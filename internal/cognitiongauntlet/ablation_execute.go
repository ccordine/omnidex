package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func executeAblation(
	ctx context.Context,
	budget RunBudget,
	environment cognition.Environment,
	completion ablationCompletionAuthority,
	recorder *EpisodeRecorder,
	state *ablationState,
	loader *ablationProjectionLoader,
	journal *ablationCallJournal,
	policy *cognitionpolicy.Policy,
	privateEvidence ContaminatedEvidencePacket,
	transition cognition.Transition,
	startedAt time.Time,
) (ablationExecution, error) {
	execution := ablationExecution{
		Revision: transition.Current,
		Planning: PlanningMetrics{ObligationsCreated: 1, PlanGenerations: 1},
	}
	recordAblationWorkingSetPeak(state, &execution.Resources)
	for cycle := 1; cycle <= budget.RuntimeCycles; cycle++ {
		if transition.Terminal {
			return completeAblation(ctx, completion, state, execution, transition, startedAt, recorder)
		}
		if execution.Resources.PolicyCallsConsumed >= budget.ModelCalls {
			return failAblation(execution, recorder, ablationTerminalCause{
				Kind: ablationTerminalPreCallBudget, Reason: "model_calls",
				CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: cycle - 1,
			}, "resource_budget", "ablation-budget-model-calls",
				"The frozen model-call budget was exhausted.", startedAt, true)
		}
		context, err := state.context(uint32(cycle), privateEvidence)
		if err != nil {
			return ablationExecution{}, err
		}
		projection, snapshot, err := prepareAblationPolicyInput(
			state, budget, context, transition.Current, uint32(cycle), execution.Resources.PolicyCallsConsumed,
		)
		if err != nil {
			if errors.Is(err, errAblationContextBudget) {
				var exact *ablationContextBudgetFailure
				if !errors.As(err, &exact) {
					return ablationExecution{}, fmt.Errorf(
						"context budget failure lacks exact failed projection authority: %w", err,
					)
				}
				execution.ContextBudget = exact.clone()
				id := fmt.Sprintf("ablation-context-budget-%03d", cycle)
				return failAblation(execution, recorder, ablationTerminalCause{
					Kind: ablationTerminalContextBudget, Reason: "context_projection",
					CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: cycle - 1,
				}, "resource_budget", id, err.Error(), startedAt, true)
			}
			return ablationExecution{}, fmt.Errorf("prepare %s policy input: %w", state.variant, err)
		}
		loader.current = projection
		callIndex := execution.Resources.PolicyCallsConsumed
		outcome, policyErr := policy.Decide(ctx, snapshot)
		if policyErr != nil && !registeredAblationPolicyFailure(policyErr) {
			return ablationExecution{}, policyErr
		}
		call, err := journal.completed(callIndex)
		if err != nil {
			return ablationExecution{}, err
		}
		if err := journal.bindInput(call.Attempt.ID, projection, snapshot); err != nil {
			return ablationExecution{}, err
		}
		if err := appendAblationPolicyTrace(recorder, projection, call, &execution.Resources); err != nil {
			return ablationExecution{}, err
		}
		if policyErr != nil {
			state.recordNoAction(
				uint32(cycle), call.Attempt.ID, ablationPolicyNoDecision,
				string(call.Result.FailureCode),
			)
			id := fmt.Sprintf("ablation-policy-failure-%03d", cycle)
			code := "model_policy"
			if errors.Is(policyErr, cognitionpolicy.ErrProviderUsageLimit) {
				code = "resource_budget"
			}
			return failAblation(execution, recorder, ablationTerminalCause{
				Kind: ablationTerminalPolicyDecision, CallOrdinal: uint32(cycle),
				Reason:         string(call.Result.FailureCode),
				CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: cycle,
			}, code, id, call.Result.FailureMessage, startedAt, false)
		}
		requestAction := outcome.Decision.Action.Clone()
		if state.variant == VariantRawShell {
			requestAction, err = parseRawShellDecision(requestAction, state.catalog)
			if err != nil {
				state.recordNoAction(
					uint32(cycle), call.Attempt.ID, ablationAcceptedNoAction,
					"raw_shell_parse_failure",
				)
				id := fmt.Sprintf("ablation-shell-failure-%03d", cycle)
				return failAblation(execution, recorder, ablationTerminalCause{
					Kind: ablationTerminalNoDispatch, CallOrdinal: uint32(cycle),
					Reason:         "raw_shell_parse_failure",
					CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: cycle,
				}, "model_policy", id, err.Error(), startedAt, false)
			}
		}
		terminal, err := applyAblationDecision(
			ctx, environment, recorder, state, &execution, outcome.Decision,
			call.Attempt.ID, requestAction, transition, cycle, budget,
		)
		if err != nil {
			return ablationExecution{}, err
		}
		transition = terminal
		if terminal.Terminal {
			return completeAblation(ctx, completion, state, execution, terminal, startedAt, recorder)
		}
		if execution.Outcome.Terminal {
			execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
			return execution, nil
		}
	}
	return failAblation(execution, recorder, ablationTerminalCause{
		Kind: ablationTerminalCycleBudget, Reason: "runtime_cycles",
		CompletedCalls: execution.Resources.PolicyCallsConsumed, CompletedCycles: budget.RuntimeCycles,
	}, "resource_budget", "ablation-budget-cycles",
		"The frozen runtime-cycle budget was exhausted.", startedAt, true)
}

func recordAblationWorkingSetPeak(state *ablationState, resources *Resources) {
	if state == nil || state.workingSet == nil || resources == nil {
		return
	}
	resident := int64(state.workingSet.Usage().ResidentBytes)
	if resident > resources.PeakWorkingSetBytes {
		resources.PeakWorkingSetBytes = resident
	}
}
