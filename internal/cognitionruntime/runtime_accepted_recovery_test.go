package cognitionruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestAcceptedDecisionRecoveryUsesReplacementAttemptWithoutNewPolicyOrSnapshot(t *testing.T) {
	harness := newRuntimeHarness(t)
	recovery := seedAcceptedRecovery(t, harness, false)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.State != StepActionSucceeded || !result.RecoveredDecision || result.RecoveredAction ||
		result.PolicyCalled || harness.policyCalls != 0 || harness.completionCalls != 1 {
		t.Fatalf("result=%#v policy=%d completion=%d", result, harness.policyCalls, harness.completionCalls)
	}
	if len(harness.applied) != 1 || harness.applied[0].Actor != harness.fixture.replacement.Attempt ||
		!reflect.DeepEqual(harness.applied[0].Request, recovery.Decision.Action) {
		t.Fatalf("recovered decision action=%#v", harness.applied)
	}
	for _, operation := range harness.order {
		if operation == "snapshot" || operation == "policy" || operation == "terminal-progress" {
			t.Fatalf("accepted recovery repeated forbidden phase: %v", harness.order)
		}
	}
}

func TestAcceptedDecisionRecoveryReplaysSameAttemptWithoutNewPolicyOrSnapshot(t *testing.T) {
	harness := newRuntimeHarness(t)
	recovery := seedAcceptedRecoveryFor(t, harness, harness.fixture.binding, false)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepActionSucceeded || !result.RecoveredDecision || result.RecoveredAction ||
		result.PolicyCalled || harness.policyCalls != 0 || harness.completionCalls != 1 {
		t.Fatalf("result=%#v policy=%d completion=%d", result, harness.policyCalls, harness.completionCalls)
	}
	if recovery.SourceActor != harness.fixture.binding.Attempt ||
		len(harness.applied) != 1 || harness.applied[0].Actor != harness.fixture.binding.Attempt {
		t.Fatalf("same-attempt recovery=%#v applied=%#v", recovery, harness.applied)
	}
	for _, operation := range harness.order {
		if operation == "snapshot" || operation == "policy" || operation == "terminal-progress" {
			t.Fatalf("same-attempt recovery repeated forbidden phase: %v", harness.order)
		}
	}
}

func TestAcceptedDecisionRecoveryUsesExistingReconciliationWithoutReapplyingProposals(t *testing.T) {
	harness := newRuntimeHarness(t)
	seedAcceptedRecovery(t, harness, true)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if !result.RecoveredDecision || result.PolicyCalled {
		t.Fatalf("result=%#v", result)
	}
	for _, operation := range harness.order {
		if operation == "reconcile" {
			t.Fatalf("existing reconciliation was repeated: %v", harness.order)
		}
	}
}

func TestAcceptedDecisionRecoveryRejectsDecisionAfterGoalBecomesSatisfied(t *testing.T) {
	harness := newRuntimeHarness(t)
	seedAcceptedRecovery(t, harness, false)
	harness.forceSatisfied = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	if !errors.Is(err, ErrInvalidJournalState) || !result.RecoveredDecision ||
		harness.policyCalls != 0 || harness.environmentCalls != 0 {
		t.Fatalf("result=%#v error=%v policy=%d environment=%d", result, err, harness.policyCalls, harness.environmentCalls)
	}
}

func TestAcceptedDecisionRecoveryRejectsStaleRecoveryActor(t *testing.T) {
	harness := newRuntimeHarness(t)
	seedAcceptedRecovery(t, harness, false)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, ErrInvalidJournalState) || harness.environmentCalls != 0 || harness.policyCalls != 0 {
		t.Fatalf("error=%v environment=%d policy=%d", err, harness.environmentCalls, harness.policyCalls)
	}
}

func TestAcceptedDecisionRecoveryRejectsDifferentWorkerAtSameAttempt(t *testing.T) {
	harness := newRuntimeHarness(t)
	prepared, err := harness.PrepareSnapshot(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	outcome, err := harness.Decide(context.Background(), prepared.Snapshot)
	requireNoError(t, err)
	decision := outcome.Decision
	target := harness.fixture.binding
	target.Attempt.WorkerID = "worker-without-exact-source-authority"

	_, err = NewAcceptedDecisionRecovery(
		target, "cognition-call-runtime-1", prepared, decision, harness.fixture.schema, nil,
	)
	if !errors.Is(err, ErrInvalidJournalState) {
		t.Fatalf("same-attempt different-worker error=%v", err)
	}
}

func seedAcceptedRecovery(
	t *testing.T,
	harness *runtimeHarness,
	withReconciliation bool,
) AcceptedDecisionRecovery {
	return seedAcceptedRecoveryFor(t, harness, harness.fixture.replacement, withReconciliation)
}

func seedAcceptedRecoveryFor(
	t *testing.T,
	harness *runtimeHarness,
	target Binding,
	withReconciliation bool,
) AcceptedDecisionRecovery {
	t.Helper()
	prepared, err := harness.PrepareSnapshot(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	outcome, err := harness.Decide(context.Background(), prepared.Snapshot)
	requireNoError(t, err)
	decision := outcome.Decision
	var reconciliation *ReconciliationReplay
	if withReconciliation {
		command := ReconciliationCommand{
			Binding: harness.fixture.binding, SnapshotSHA256: prepared.Snapshot.SHA256(),
			Projection: prepared.Snapshot.ContextProjection(), ActionSchema: harness.fixture.schema,
			Decision: decision,
		}
		value, err := NewReconciliationReceipt(command, harness.version, prepared.Snapshot.ContextProjection().WorkingSetVersion)
		requireNoError(t, err)
		reconciliation = &ReconciliationReplay{Command: command, Receipt: value}
	}
	recovery, err := NewAcceptedDecisionRecovery(
		target, "cognition-call-runtime-1", prepared, decision,
		harness.fixture.schema, reconciliation,
	)
	requireNoError(t, err)
	harness.acceptedRecovery = &recovery
	harness.order = nil
	harness.policyCalls, harness.completionCalls = 0, 0
	return recovery
}

var _ cognition.Policy = (*runtimeHarness)(nil)
