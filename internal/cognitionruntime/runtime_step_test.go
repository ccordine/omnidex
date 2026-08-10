package cognitionruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestStepEvaluatesCodeCompletionBeforePolicyAndSeals(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.forceSatisfied = true
	harness.terminal = true
	harness.public = "The registered goal state was reached."
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepEpisodeCompleted || result.Seal == nil || result.PolicyCalled {
		t.Fatalf("result = %#v", result)
	}
	if harness.policyCalls != 0 || harness.environmentCalls != 0 || harness.completionCalls != 1 {
		t.Fatalf(
			"calls: policy=%d environment=%d completion=%d",
			harness.policyCalls, harness.environmentCalls, harness.completionCalls,
		)
	}
	wantOrder := []string{
		"unresolved", "accepted-recovery", "terminal-progress", "terminal-policy", "abandon-policy",
		"snapshot", "completion", "advance", "seal",
	}
	if !reflect.DeepEqual(harness.order, wantOrder) {
		t.Fatalf("order=%v want=%v", harness.order, wantOrder)
	}
}

func TestSatisfiedChildAdvancesAndActivatesExactlyOneObligation(t *testing.T) {
	harness := newRuntimeHarness(t)
	childID := configureChildObligation(t, harness)
	harness.forceSatisfied = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepObligationAdvanced || result.Completion == nil ||
		result.Completion.ObligationID != childID || harness.policyCalls != 0 {
		t.Fatalf("result=%#v policy calls=%d", result, harness.policyCalls)
	}
	active := 0
	for _, obligation := range harness.graph.Obligations {
		if obligation.Status == cognition.ObligationActive {
			active++
			if obligation.ID != harness.graph.RootID {
				t.Fatalf("unexpected active obligation %q", obligation.ID)
			}
		}
	}
	if active != 1 {
		t.Fatalf("active obligations=%d graph=%#v", active, harness.graph)
	}
}

func TestTerminalEnvironmentWithUnsatisfiedPredicateFailsAndSeals(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.terminal, harness.forceUnsatisfied = true, true
	harness.public = "The environment cannot accept another operation."
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepEpisodeFailed || result.Seal == nil || result.PolicyCalled ||
		result.Completion == nil || result.Completion.Outcome != cognition.CompletionUnsatisfied {
		t.Fatalf("result = %#v", result)
	}
	if harness.policyCalls != 0 || harness.environmentCalls != 0 {
		t.Fatalf("terminal failure reached policy/environment: %v", harness.order)
	}
}

func TestStepJournalsOneBoundDecisionBeforeEnvironmentMutation(t *testing.T) {
	harness := newRuntimeHarness(t)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepActionSucceeded || !result.PolicyCalled || result.RecoveredAction ||
		result.EnvironmentActions != 1 || result.Transition == nil {
		t.Fatalf("result = %#v", result)
	}
	wantOrder := []string{
		"unresolved", "accepted-recovery", "terminal-progress", "terminal-policy", "abandon-policy",
		"snapshot", "completion", "policy", "reconcile",
		"prepare-action", "dispatch", "environment", "transition",
	}
	if !reflect.DeepEqual(harness.order, wantOrder) {
		t.Fatalf("order=%v want=%v", harness.order, wantOrder)
	}
	if len(harness.applied) != 1 || harness.applied[0].Actor != harness.fixture.binding.Attempt {
		t.Fatalf("applied actions = %#v", harness.applied)
	}
}

func TestStepPersistsTypedActionFailure(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.typedFailureCode = cognition.ActionFailurePreconditionFailed
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepActionFailed || result.Failure == nil ||
		result.Failure.Code != cognition.ActionFailurePreconditionFailed || harness.unresolved != nil {
		t.Fatalf("result=%#v unresolved=%#v", result, harness.unresolved)
	}
	if harness.order[len(harness.order)-1] != "failure" {
		t.Fatalf("failure was not the final durable write: %v", harness.order)
	}
}

func TestStepLeavesDispatchedActionForUnknownEnvironmentError(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.environmentError = errors.New("transport unavailable")
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, ErrEnvironment) {
		t.Fatalf("error=%v want ErrEnvironment", err)
	}
	if harness.unresolved == nil || harness.unresolved.Status != ActionDispatched {
		t.Fatalf("dispatched action was not retained: %#v", harness.unresolved)
	}
	for _, operation := range harness.order {
		if operation == "failure" || operation == "transition" {
			t.Fatalf("unknown error was persisted as a typed result: %v", harness.order)
		}
	}
}

func TestRunContinuesFromActionToCodeOwnedTerminalSeal(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.nextTerminal = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.binding, RunLimits{MaxCycles: 2})
	requireNoError(t, err)
	if result.Cycles != 2 || result.PolicyCalls != 1 || result.EnvironmentActions != 1 ||
		result.Terminal.State != StepEpisodeCompleted {
		t.Fatalf("run result = %#v", result)
	}
}

func TestRunFailsLoudlyAtExplicitCycleLimit(t *testing.T) {
	harness := newRuntimeHarness(t)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.binding, RunLimits{MaxCycles: 1})
	if !errors.Is(err, ErrRunCycleLimit) || result.Cycles != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestRunRetainsPartialCallAndEnvironmentCountsOnJournalFailure(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.transitionWriteFailures = 1
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.binding, RunLimits{MaxCycles: 2})
	if err == nil {
		t.Fatal("run accepted a failed transition journal write")
	}
	if result.Cycles != 1 || result.PolicyCalls != 1 || result.EnvironmentActions != 1 {
		t.Fatalf("partial run metrics = %#v", result)
	}
}
