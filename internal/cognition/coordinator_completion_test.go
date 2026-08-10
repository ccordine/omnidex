package cognition

import (
	"context"
	"errors"
	"testing"
)

func TestCoordinatorUsesCodeOwnedCompletionWithoutCallingPolicy(t *testing.T) {
	t.Parallel()
	snapshot, _, evidence := testRuntimeSnapshot(t)
	policyCalls := 0
	coordinator, err := NewCoordinator(PolicyFunc(func(context.Context, RuntimeSnapshot) (PolicyOutcome, error) {
		policyCalls++
		return PolicyOutcome{InferenceExecuted: true}, errors.New("must not be called")
	}))
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewCompletionResult(
		snapshot.CurrentObligation().ID,
		snapshot.CurrentObligation().CompletionCheck,
		snapshot.CurrentRevision(),
		CompletionSatisfied,
		[]EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(context.Background(), snapshot, completion, []EvidenceRef{evidence})
	if err != nil {
		t.Fatalf("completion step: %v", err)
	}
	if policyCalls != 0 || step.State != CoordinatorObligationSatisfied || step.Decision != nil {
		t.Fatalf("completion step = %#v, policy calls=%d", step, policyCalls)
	}
}

func TestCoordinatorAcceptsCompletionEvidenceFromExactSnapshotPacket(t *testing.T) {
	t.Parallel()
	snapshot, _, evidence := testRuntimeSnapshot(t)
	current := snapshot.CurrentObligation()
	current.SupportingRefs = []EvidenceRef{}
	rebuilt, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), current, snapshot.ActionCatalog(),
		snapshot.Attempt(), snapshot.ContextProjection(), snapshot.Budget(), []EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	policyCalls := 0
	coordinator, err := NewCoordinator(PolicyFunc(func(context.Context, RuntimeSnapshot) (PolicyOutcome, error) {
		policyCalls++
		return PolicyOutcome{InferenceExecuted: true}, errors.New("must not be called")
	}))
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewCompletionResult(
		current.ID, current.CompletionCheck, rebuilt.CurrentRevision(),
		CompletionSatisfied, []EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(context.Background(), rebuilt, completion, []EvidenceRef{evidence})
	if err != nil {
		t.Fatalf("completion packet evidence rejected: %v", err)
	}
	if policyCalls != 0 || step.State != CoordinatorObligationSatisfied {
		t.Fatalf("step = %#v, policy calls=%d", step, policyCalls)
	}
}
