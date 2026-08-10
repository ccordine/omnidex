package cognitionruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestRuntimeRequiresEveryAuthorityPort(t *testing.T) {
	harness := newRuntimeHarness(t)
	base := harness.dependencies()
	cases := []struct {
		name string
		drop func(*Dependencies)
	}{
		{"policy", func(value *Dependencies) { value.Policy = nil }},
		{"environment", func(value *Dependencies) { value.Environment = nil }},
		{"snapshots", func(value *Dependencies) { value.Snapshots = nil }},
		{"policy recovery", func(value *Dependencies) { value.PolicyRecovery = nil }},
		{"completion", func(value *Dependencies) { value.Completion = nil }},
		{"episodes", func(value *Dependencies) { value.Episodes = nil }},
		{"reconciler", func(value *Dependencies) { value.Reconciler = nil }},
		{"actions", func(value *Dependencies) { value.Actions = nil }},
		{"terminal seal", func(value *Dependencies) { value.TerminalSeal = nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dependencies := base
			test.drop(&dependencies)
			if _, err := New(dependencies); !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("error=%v want ErrInvalidConfiguration", err)
			}
		})
	}
}

func TestPolicyBudgetExhaustionOccursAfterCompletionCheckAndBeforeModelCall(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.remainingCalls = 0
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, cognition.ErrCoordinatorBudgetExhausted) {
		t.Fatalf("error=%v want budget exhaustion", err)
	}
	if harness.completionCalls != 1 || harness.policyCalls != 0 || harness.environmentCalls != 0 {
		t.Fatalf(
			"calls: completion=%d policy=%d environment=%d",
			harness.completionCalls, harness.policyCalls, harness.environmentCalls,
		)
	}
}

func TestChangedReconciliationReceiptCannotReachActionJournal(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.corruptReceipt = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, ErrInvalidJournalState) {
		t.Fatalf("error=%v want journal authority failure", err)
	}
	if harness.unresolved != nil || harness.environmentCalls != 0 {
		t.Fatalf("changed receipt reached mutation: action=%#v calls=%d", harness.unresolved, harness.environmentCalls)
	}
}

func TestReconciliationReceiptRejectsUnpersistedWorkingSetVersion(t *testing.T) {
	harness := newRuntimeHarness(t)
	prepared, err := harness.PrepareSnapshot(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	outcome, err := harness.Decide(context.Background(), prepared.Snapshot)
	requireNoError(t, err)
	decision := outcome.Decision
	command := ReconciliationCommand{
		Binding: harness.fixture.binding, SnapshotSHA256: prepared.Snapshot.SHA256(),
		Projection: prepared.Snapshot.ContextProjection(), ActionSchema: harness.fixture.schema,
		Decision: decision,
	}
	_, err = NewReconciliationReceipt(
		command, 1, command.Projection.WorkingSetVersion-1,
	)
	if !errors.Is(err, ErrInvalidJournalState) {
		t.Fatalf("error=%v want persisted-version rejection", err)
	}
}

func TestReconcilerCannotMutateTheCoordinatorDecisionByAliasing(t *testing.T) {
	harness := newRuntimeHarness(t)
	dependencies := harness.dependencies()
	dependencies.Reconciler = mutatingReconciler{}
	runtime, err := New(dependencies)
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, ErrInvalidJournalState) {
		t.Fatalf("error=%v want immutable receipt rejection", err)
	}
	if harness.unresolved != nil || harness.environmentCalls != 0 {
		t.Fatalf("mutated decision reached action authority")
	}
}

func TestResolvedJournalRecordCannotChangePreparedActionContent(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.corruptResolvedAction = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, ErrInvalidJournalState) {
		t.Fatalf("error=%v want immutable action rejection", err)
	}
}

func TestStepRejectsNilContextAndInvalidBinding(t *testing.T) {
	harness := newRuntimeHarness(t)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	if _, err := runtime.Step(nil, harness.fixture.binding); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("nil context error=%v", err)
	}
	invalid := harness.fixture.binding
	invalid.Attempt.WorkerID = ""
	if _, err := runtime.Step(context.Background(), invalid); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("invalid binding error=%v", err)
	}
}

type mutatingReconciler struct{}

func (mutatingReconciler) Reconcile(
	_ context.Context,
	command ReconciliationCommand,
) (ReconciliationReceipt, error) {
	command.Decision.Action.Arguments[0].Value = "changed"
	return NewReconciliationReceipt(command, 1, command.Projection.WorkingSetVersion)
}
