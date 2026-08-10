package cognitionruntime

import (
	"context"
	"errors"
	"testing"
)

func TestRunCountsFailedProviderInvocationFromExactPolicyOutcome(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.policyError = errors.New("provider invocation failed")
	harness.policyProviderRequestDispatched = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.binding, RunLimits{MaxCycles: 1})
	if err == nil || result.Cycles != 1 || result.PolicyCalls != 1 || harness.policyCalls != 1 {
		t.Fatalf("result=%#v error=%v policy calls=%d", result, err, harness.policyCalls)
	}
}

func TestRunDoesNotCountDurablePolicyResultReplayAsInference(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.policyError = errors.New("durable rejected call replay")
	harness.policyProviderRequestDispatched = false
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Run(context.Background(), harness.fixture.binding, RunLimits{MaxCycles: 1})
	if err == nil || result.Cycles != 1 || result.PolicyCalls != 0 || harness.policyCalls != 1 {
		t.Fatalf("result=%#v error=%v policy boundary calls=%d", result, err, harness.policyCalls)
	}
}
