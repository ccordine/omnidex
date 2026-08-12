package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayAllowsCompletionSnapshotsAtTheSameNextCallOrdinal(t *testing.T) {
	state := semanticReplayCallClosureFixture()
	if err := state.finishPreparedInputs(); err != nil {
		t.Fatal(err)
	}
	if err := state.finishPolicyCalls(); err != nil {
		t.Fatal(err)
	}
	if err := state.consumeSnapshot("snapshot-progress-one", "policy-call://call-1"); err == nil {
		t.Fatal("one runtime snapshot was consumed by both progress and policy")
	}
}

func TestSemanticReplayCallClosureRejectsOrphansAndWrongDownstreamWork(t *testing.T) {
	for name, mutate := range map[string]func(*semanticReplayState){
		"orphan projection": func(state *semanticReplayState) {
			delete(state.projectionConsumers, "projection-progress-one")
		},
		"missing timing": func(state *semanticReplayState) {
			delete(state.policyTimings, "call-1")
		},
		"missing action": func(state *semanticReplayState) {
			delete(state.callActions, "call-1")
		},
		"rejected with downstream work": func(state *semanticReplayState) {
			result := state.results["call-1"]
			result.Status = cognitionpolicy.CallResultRejected
			state.results["call-1"] = result
			timing := state.policyTimings["call-1"]
			timing.Status = cognitionpolicy.CallResultRejected
			state.policyTimings["call-1"] = timing
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := semanticReplayCallClosureFixture()
			mutate(state)
			if state.finishPreparedInputs() == nil && state.finishPolicyCalls() == nil {
				t.Fatal("changed semantic call closure was accepted")
			}
		})
	}
}

func TestSemanticReplayAllowsPreparedSnapshotWithoutAConsumer(t *testing.T) {
	state := semanticReplayCallClosureFixture()
	delete(state.snapshotConsumers, "snapshot-progress-one")
	if err := state.finishPreparedInputs(); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticReplayAllowsOnlyLifecycleCanceledAcceptedSuffixes(t *testing.T) {
	for name, code := range map[string]cognitionruntime.CancellationCode{
		"job canceled":          cognitionruntime.CancellationJobCanceled,
		"generation retired":    cognitionruntime.CancellationGenerationRetired,
		"worker policy failure": cognitionruntime.CancellationPolicyFailure,
	} {
		t.Run(name, func(t *testing.T) {
			state := semanticReplayCallClosureFixture()
			state.cancellation = &cognitionruntime.CancellationEvidence{Code: code}
			delete(state.callActions, "call-1")
			delete(state.actions, cognition.ActionID("action-1"))
			delete(state.callReconciliations, "call-1")
			delete(state.reconciliationCalls, "reconciliation-1")
			delete(state.reconciles, "reconciliation-1")
			delete(state.reconciliationReceipts, "reconciliation-1")
			err := state.finishPolicyCalls()
			if (code == cognitionruntime.CancellationPolicyFailure) == (err == nil) {
				t.Fatalf("partial accepted suffix code=%q error=%v", code, err)
			}
		})
	}
	state := semanticReplayCallClosureFixture()
	state.cancellation = &cognitionruntime.CancellationEvidence{
		Code: cognitionruntime.CancellationJobCanceled,
	}
	delete(state.callActions, "call-1")
	delete(state.actions, cognition.ActionID("action-1"))
	if err := state.finishPolicyCalls(); err != nil {
		t.Fatalf("lifecycle cancellation rejected committed reconciliation before action: %v", err)
	}
}

func semanticReplayCallClosureFixture() *semanticReplayState {
	callID := "call-1"
	reconciliationID := "reconciliation-1"
	actionID := cognition.ActionID("action-1")
	state := &semanticReplayState{
		projections: map[cognition.ContextProjectionID]semanticProjectionRecord{
			"projection-progress-one": {callOrdinal: 1},
			"projection-progress-two": {callOrdinal: 1},
			"projection-policy":       {callOrdinal: 1},
		},
		projectionConsumers: map[cognition.ContextProjectionID]string{
			"projection-progress-one": "snapshot-progress-one",
			"projection-progress-two": "snapshot-progress-two",
			"projection-policy":       "snapshot-policy",
		},
		snapshots: map[string]semanticSnapshotRecord{
			"snapshot-progress-one": {callOrdinal: 1},
			"snapshot-progress-two": {callOrdinal: 1},
			"snapshot-policy":       {callOrdinal: 1},
		},
		snapshotConsumers: map[string]string{
			"snapshot-progress-one": "episode-progress://progress-one",
			"snapshot-progress-two": "episode-progress://progress-two",
			"snapshot-policy":       "policy-call://" + callID,
		},
		attempts: map[string]cognitionpolicy.CallAttempt{
			callID: {ID: callID, SnapshotSHA256: "snapshot-policy"},
		},
		attemptOrdinals: map[int64]string{1: callID},
		results: map[string]cognitionpolicy.CallResult{
			callID: {CallID: callID, Status: cognitionpolicy.CallResultAccepted},
		},
		abandoned: map[string]struct{}{},
		policyTimings: map[string]queue.CognitionTracePolicyTiming{
			callID: {CallID: callID, Status: cognitionpolicy.CallResultAccepted},
		},
		reconciles: map[string]cognitionruntime.ReconciliationCommand{
			reconciliationID: {SnapshotSHA256: "snapshot-policy"},
		},
		reconciliationReceipts: map[string]cognitionruntime.ReconciliationReceipt{
			reconciliationID: {},
		},
		reconciliationCalls: map[string]string{reconciliationID: callID},
		callReconciliations: map[string]string{callID: reconciliationID},
		actions: map[cognition.ActionID]queue.CognitionTraceAction{
			actionID: {PolicyCallID: callID, ReconciliationID: reconciliationID},
		},
		callActions: map[string]cognition.ActionID{callID: actionID},
	}
	return state
}
