package objectiveworkload_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

func TestRunReturnsHonestPartialStateOnCodeOperationFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		operations *scriptedOperations
		wantCalls  int
		wantTrace  int
		wantArts   int
	}{
		{
			name:       "materialize",
			operations: &scriptedOperations{materializeErr: errors.New("materializer stopped")},
			wantCalls:  1,
		},
		{
			name:       "verify",
			operations: &scriptedOperations{verifyErr: errors.New("verifier rejected")},
			wantCalls:  2, wantTrace: 1, wantArts: 1,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			workload := compileOne(t)
			result, err := objectiveworkload.Run(
				context.Background(), workload, test.operations,
				objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
			)
			if !errors.Is(err, objectiveworkload.ErrOperation) {
				t.Fatalf("error=%v", err)
			}
			if result.Complete || result.WorkloadID != workload.ID ||
				result.DeterministicOperationCalls != test.wantCalls ||
				len(result.Trace) != test.wantTrace || len(result.Artifacts) != test.wantArts ||
				result.StationCalls != 0 || result.ModelCalls != 0 {
				t.Fatalf("partial result=%+v", result)
			}
		})
	}
}

func TestRunRejectsInvalidEntryWithoutPlausibleWorkloadAuthority(t *testing.T) {
	t.Parallel()
	valid := compileOne(t)
	tests := []struct {
		name       string
		workload   objectiveworkload.Workload
		operations objectiveworkload.Operations
		limits     objectiveworkload.RunLimits
	}{
		{
			name: "invalid graph", workload: objectiveworkload.Workload{ID: valid.ID},
			operations: &scriptedOperations{}, limits: objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
		},
		{
			name: "invalid limits", workload: valid,
			operations: &scriptedOperations{}, limits: objectiveworkload.RunLimits{},
		},
		{
			name: "nil operations", workload: valid,
			operations: nil, limits: objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := objectiveworkload.Run(
				context.Background(), test.workload, test.operations, test.limits,
			)
			if err == nil {
				t.Fatal("invalid run entry unexpectedly succeeded")
			}
			if result.WorkloadID != "" || result.Complete || len(result.Trace) != 0 || len(result.Artifacts) != 0 {
				t.Fatalf("invalid entry returned plausible authority: %+v", result)
			}
		})
	}
}

func TestRunRequiresAcceptanceUnsatisfiedAtEntry(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	workload.Objectives[0].Status = objectiveworkload.ObjectiveComplete
	operations := &scriptedOperations{}
	result, err := objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if err == nil {
		t.Fatal("premature completion status unexpectedly accepted")
	}
	if result.WorkloadID != "" || len(operations.calls) != 0 {
		t.Fatalf("result=%+v calls=%v", result, operations.calls)
	}
}
