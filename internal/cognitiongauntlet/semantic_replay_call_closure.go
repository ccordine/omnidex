package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (state *semanticReplayState) consumeProjection(
	id cognition.ContextProjectionID,
	consumer string,
) error {
	if prior, used := state.projectionConsumers[id]; used {
		return fmt.Errorf("semantic Context Projection is reused by %q", prior)
	}
	state.projectionConsumers[id] = consumer
	return nil
}

func (state *semanticReplayState) consumeSnapshot(sha, consumer string) error {
	if prior, used := state.snapshotConsumers[sha]; used {
		return fmt.Errorf("semantic runtime snapshot is reused by %q", prior)
	}
	state.snapshotConsumers[sha] = consumer
	return nil
}

func (state *semanticReplayState) finishPreparedInputs() error {
	if len(state.projectionConsumers) != len(state.projections) {
		return fmt.Errorf("semantic Context Projections are not consumed exactly once")
	}
	for id := range state.projections {
		if state.projectionConsumers[id] == "" {
			return fmt.Errorf("semantic Context Projection %q is orphaned", id)
		}
	}
	for sha := range state.snapshotConsumers {
		if _, exists := state.snapshots[sha]; !exists {
			return fmt.Errorf("semantic runtime snapshot consumer cites unknown snapshot %q", sha)
		}
	}
	return nil
}

func (state *semanticReplayState) finishPolicyCalls() error {
	if len(state.attemptOrdinals) != len(state.attempts) ||
		len(state.policyTimings) != len(state.attempts) {
		return fmt.Errorf("semantic policy call ordinals or timings are incomplete")
	}
	for ordinal := int64(1); ordinal <= int64(len(state.attempts)); ordinal++ {
		callID, exists := state.attemptOrdinals[ordinal]
		if !exists {
			return fmt.Errorf("semantic policy call ordinal %d is missing", ordinal)
		}
		attempt, exists := state.attempts[callID]
		if !exists || state.snapshotConsumers[attempt.SnapshotSHA256] != "policy-call://"+callID {
			return fmt.Errorf("semantic policy call %q lacks its exact consumed snapshot", callID)
		}
		result, hasResult := state.results[callID]
		_, isAbandoned := state.abandoned[callID]
		if hasResult == isAbandoned {
			return fmt.Errorf("semantic policy call %q lacks exactly one terminal outcome", callID)
		}
		timing, hasTiming := state.policyTimings[callID]
		if !hasTiming || (hasResult && timing.Status != result.Status) ||
			(isAbandoned && string(timing.Status) != "abandoned") {
			return fmt.Errorf("semantic policy call %q timing differs from its terminal outcome", callID)
		}
		reconciliationID, hasReconciliation := state.callReconciliations[callID]
		actionID, hasAction := state.callActions[callID]
		accepted := hasResult && result.Status == cognitionpolicy.CallResultAccepted
		if accepted {
			command, commandExists := state.reconciles[reconciliationID]
			_, receiptExists := state.reconciliationReceipts[reconciliationID]
			action, actionExists := state.actions[actionID]
			if state.lifecycleCancellationAllowsPartial(callID) && !hasReconciliation && !hasAction {
				continue
			}
			if state.lifecycleCancellationAllowsPartial(callID) && hasReconciliation &&
				commandExists && receiptExists && !hasAction &&
				state.reconciliationCalls[reconciliationID] == callID &&
				command.SnapshotSHA256 == attempt.SnapshotSHA256 {
				continue
			}
			if !hasReconciliation || !commandExists || !receiptExists || !hasAction || !actionExists ||
				state.reconciliationCalls[reconciliationID] != callID ||
				action.PolicyCallID != callID || action.ReconciliationID != reconciliationID ||
				command.SnapshotSHA256 != attempt.SnapshotSHA256 {
				return fmt.Errorf("accepted semantic policy call %q lacks one exact action chain", callID)
			}
		} else if hasReconciliation || hasAction {
			return fmt.Errorf("non-accepted semantic policy call %q has downstream action work", callID)
		}
	}
	if len(state.reconciliationCalls) != len(state.reconciles) ||
		len(state.callReconciliations) != len(state.reconciles) ||
		len(state.reconciliationReceipts) != len(state.reconciles) ||
		len(state.callActions) != len(state.actions) {
		return fmt.Errorf("semantic policy downstream authority is not one-to-one")
	}
	return nil
}

func (state *semanticReplayState) lifecycleCancellationAllowsPartial(callID string) bool {
	if state.cancellation == nil || state.attemptOrdinals[int64(len(state.attempts))] != callID {
		return false
	}
	return state.cancellation.Code == cognitionruntime.CancellationJobCanceled ||
		state.cancellation.Code == cognitionruntime.CancellationGenerationRetired
}
