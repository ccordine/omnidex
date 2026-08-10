package cognitionruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPreparedActionRecoveryDispatchesWithoutCallingPolicy(t *testing.T) {
	harness := newRuntimeHarness(t)
	original := seedUnresolvedAction(t, harness, harness.fixture.binding, ActionPrepared)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.State != StepActionSucceeded || !result.RecoveredAction || result.PolicyCalled ||
		harness.policyCalls != 0 || harness.completionCalls != 0 {
		t.Fatalf("result=%#v policy=%d completion=%d", result, harness.policyCalls, harness.completionCalls)
	}
	if len(harness.applied) != 1 || harness.applied[0].ID != original.Action.ID ||
		harness.applied[0].Actor != harness.fixture.replacement.Attempt ||
		!sameAppliedAction(harness.applied[0], original.Action) {
		t.Fatalf("recovered action changed: applied=%#v original=%#v", harness.applied, original.Action)
	}
}

func TestLifecycleCanceledProgressRecoversWithoutAnyModelEnvironmentActionOrSeal(t *testing.T) {
	harness := newRuntimeHarness(t)
	root := harness.graph.Obligations[0]
	completion, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, harness.fixture.revision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	requireNoError(t, err)
	evidence, err := NewLifecycleCancellationEvidence(
		CancellationGenerationRetired,
		"The owning job generation was superseded.", runtimeDigest("lifecycle-operation"),
	)
	requireNoError(t, err)
	cancellation := CancellationSeal{
		Episode: harness.fixture.binding.Episode, Code: evidence.Code,
		SourceEvidenceID: evidence.ID, TraceSHA256: runtimeDigest("canceled-trace"),
	}
	harness.terminalProgress = &EpisodeProgress{
		Episode: harness.fixture.binding.Episode, State: ProgressCanceled,
		Revision: harness.fixture.revision, GraphVersion: harness.version,
		ObligationGraph: harness.graph.Clone(), Completion: &completion,
		Cancellation: &cancellation, PublicOutcome: evidence.PublicMessage,
	}
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.State != StepEpisodeCanceled || result.Cancellation == nil ||
		*result.Cancellation != cancellation || !result.RecoveredProgress || result.PolicyCalled ||
		result.EnvironmentActions != 0 || harness.policyCalls != 0 ||
		harness.completionCalls != 0 || harness.environmentCalls != 0 {
		t.Fatalf("canceled recovery result=%#v calls=%d/%d/%d", result,
			harness.policyCalls, harness.completionCalls, harness.environmentCalls)
	}
	wantOrder := []string{"unresolved", "accepted-recovery", "terminal-progress"}
	if !reflect.DeepEqual(harness.order, wantOrder) {
		t.Fatalf("canceled recovery order=%v want=%v", harness.order, wantOrder)
	}
}

func TestDispatchedActionRecoveryReplaysExactIDWithoutRedispatch(t *testing.T) {
	harness := newRuntimeHarness(t)
	original := seedUnresolvedAction(t, harness, harness.fixture.binding, ActionDispatched)
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.ActionID != original.Action.ID || !result.RecoveredAction {
		t.Fatalf("result = %#v", result)
	}
	for _, operation := range harness.order {
		if operation == "dispatch" || operation == "policy" || operation == "snapshot" {
			t.Fatalf("dispatched recovery repeated a forbidden phase: %v", harness.order)
		}
	}
}

func TestCommittedDispatchCrashRecoversUnderReplacementAttempt(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.dispatchCommitError = true
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if err == nil || harness.unresolved == nil || harness.unresolved.Status != ActionDispatched {
		t.Fatalf("first step error=%v unresolved=%#v", err, harness.unresolved)
	}
	actionID := harness.unresolved.Action.ID
	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.ActionID != actionID || !result.RecoveredAction || harness.policyCalls != 1 {
		t.Fatalf("recovered result=%#v policy calls=%d", result, harness.policyCalls)
	}
}

func TestCrashBeforeTransitionWriteReplaysSameEnvironmentAction(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.transitionWriteFailures = 1
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.binding)
	if err == nil || harness.unresolved == nil || harness.unresolved.Status != ActionDispatched {
		t.Fatalf("first step error=%v unresolved=%#v", err, harness.unresolved)
	}
	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if !result.RecoveredAction || len(harness.applied) != 2 {
		t.Fatalf("result=%#v applied=%#v", result, harness.applied)
	}
	if harness.applied[0].ID != harness.applied[1].ID ||
		!sameAppliedAction(harness.applied[0], harness.applied[1]) {
		t.Fatalf("environment replay changed action: %#v", harness.applied)
	}
	if harness.applied[1].Actor != harness.fixture.replacement.Attempt {
		t.Fatalf("replacement actor was not fenced: %#v", harness.applied[1].Actor)
	}
}

func TestRecoveryRejectsActionOwnedByAnotherStepBeforeEnvironmentCall(t *testing.T) {
	harness := newRuntimeHarness(t)
	seedUnresolvedAction(t, harness, harness.fixture.binding, ActionDispatched)
	harness.unresolved.Action.Actor.StepID++
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	_, err = runtime.Step(context.Background(), harness.fixture.replacement)
	if !errors.Is(err, ErrInvalidJournalState) || harness.environmentCalls != 0 {
		t.Fatalf("error=%v environment calls=%d", err, harness.environmentCalls)
	}
}

func TestTerminalProgressRecoversSealWithoutSnapshotOrPolicyReplay(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.forceSatisfied = true
	harness.terminal = true
	harness.public = "The registered goal state was reached."
	harness.sealFailures = 1
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	if _, err := runtime.Step(context.Background(), harness.fixture.binding); err == nil ||
		harness.terminalProgress == nil {
		t.Fatalf("first terminal step error=%v progress=%#v", err, harness.terminalProgress)
	}
	policyCalls, completionCalls := harness.policyCalls, harness.completionCalls
	harness.order = nil
	result, err := runtime.Step(context.Background(), harness.fixture.replacement)
	requireNoError(t, err)
	if result.State != StepEpisodeCompleted || result.Seal == nil || !result.RecoveredProgress ||
		result.PolicyCalled || result.EnvironmentActions != 0 {
		t.Fatalf("recovered terminal result=%#v", result)
	}
	if harness.policyCalls != policyCalls || harness.completionCalls != completionCalls {
		t.Fatalf("terminal recovery repeated policy/completion: policy=%d completion=%d", harness.policyCalls, harness.completionCalls)
	}
	wantOrder := []string{"unresolved", "accepted-recovery", "terminal-progress", "seal"}
	if !reflect.DeepEqual(harness.order, wantOrder) {
		t.Fatalf("terminal recovery order=%v want=%v", harness.order, wantOrder)
	}
}
