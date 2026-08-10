package cognitionruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func TestStepReplaysDurableTerminalPolicyOutcomeWithoutFreshWork(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.terminalPolicyRecovered = true
	harness.terminalPolicyError = cognitionpolicy.ErrInvalidDecision
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	if !errors.Is(err, cognitionpolicy.ErrInvalidDecision) || !result.RecoveredPolicyOutcome ||
		result.PolicyCalled || harness.policyCalls != 0 || harness.completionCalls != 0 {
		t.Fatalf("result=%#v error=%v policy=%d completion=%d", result, err, harness.policyCalls, harness.completionCalls)
	}
	want := []string{"unresolved", "accepted-recovery", "terminal-progress", "terminal-policy"}
	if !reflect.DeepEqual(harness.order, want) {
		t.Fatalf("order=%v want=%v", harness.order, want)
	}
}

func TestStepRejectsInvalidTerminalPolicyRecoveryTuples(t *testing.T) {
	tests := []struct {
		name      string
		recovered bool
		err       error
	}{
		{name: "recovered_without_error", recovered: true},
		{name: "error_without_recovery", err: cognitionpolicy.ErrGeneration},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newRuntimeHarness(t)
			harness.terminalPolicyRecovered, harness.terminalPolicyError = test.recovered, test.err
			runtime, err := New(harness.dependencies())
			requireNoError(t, err)
			result, err := runtime.Step(context.Background(), harness.fixture.binding)
			if !errors.Is(err, ErrInvalidJournalState) || result.PolicyCalled || harness.policyCalls != 0 {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
}

func TestStepReportsExactAbandonmentAndContinuesWithFreshCall(t *testing.T) {
	harness := newRuntimeHarness(t)
	abandonment, err := NewPolicyCallAbandonment(
		harness.fixture.binding.Episode, "cognition_call_"+runtimeDigest("call"), 1,
		runtimeDigest("attempt"), runtimeDigest("snapshot"), harness.fixture.binding.Attempt,
		SourceAttemptExpired, harness.fixture.replacement.Attempt,
	)
	requireNoError(t, err)
	harness.abandonment = &abandonment
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.PolicyCallAbandonment == nil || *result.PolicyCallAbandonment != abandonment.Ref() ||
		result.AbandonedPolicyCalls != 1 || !result.PolicyCalled || harness.policyCalls != 1 {
		t.Fatalf("result=%#v calls=%d", result, harness.policyCalls)
	}
}

func TestRunCountsAbandonmentAlongsideActualInference(t *testing.T) {
	harness := newRuntimeHarness(t)
	abandonment, err := NewPolicyCallAbandonment(
		harness.fixture.binding.Episode, "cognition_call_"+runtimeDigest("call"), 1,
		runtimeDigest("attempt"), runtimeDigest("snapshot"), harness.fixture.binding.Attempt,
		SourceAttemptSuperseded, harness.fixture.replacement.Attempt,
	)
	requireNoError(t, err)
	harness.abandonment = &abandonment
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.replacement, RunLimits{MaxCycles: 1})
	if !errors.Is(err, ErrRunCycleLimit) || result.AbandonedPolicyCalls != 1 || result.PolicyCalls != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
