package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPausedInferenceCheckpointBindsCodeOwnedPrefixAndPreCallState(t *testing.T) {
	t.Parallel()
	episode := cognition.EpisodeRef{ID: "episode-takeover"}
	checkpoint, err := NewPausedInferenceCheckpoint(
		digestForTest("public-authority"), episode,
		testSemanticPreCallCheckpoint(1, "worker-before", "projection-before", "snapshot-before"),
		cognitionruntime.RunResult{Cycles: 2, PolicyCalls: 2, EnvironmentActions: 2}, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Prefix.Cycles != 2 || checkpoint.SuccessfulActions != 1 {
		t.Fatalf("paused checkpoint=%+v", checkpoint)
	}

	changed := checkpoint
	changed.SuccessfulActions = 3
	if err := changed.Validate(); err == nil {
		t.Fatal("paused checkpoint accepted impossible successful-action count")
	}
	changed = checkpoint
	changed.PreCall.Bound.SnapshotSHA256 = "invalid"
	if err := changed.Validate(); err == nil {
		t.Fatal("paused checkpoint accepted invalid pre-call authority")
	}
}

func TestControlledRuntimePrefixAccountsForEveryRecoveryPath(t *testing.T) {
	t.Parallel()
	var run cognitionruntime.RunResult
	accumulateRuntimeStep(&run, cognitionruntime.StepResult{
		PolicyCalled: true, RecoveredDecision: true, RecoveredAction: true,
		RecoveredProgress: true, RecoveredPolicyOutcome: true,
		AbandonedPolicyCalls: 1, EnvironmentActions: 1,
	})
	want := RuntimePrefix{
		Cycles: 1, PolicyCalls: 1, RecoveredDecisions: 1, RecoveredActions: 1,
		RecoveredProgress: 1, RecoveredPolicyOutcomes: 1,
		AbandonedPolicyCalls: 1, EnvironmentActions: 1,
	}
	if got := runtimePrefix(run); got != want {
		t.Fatalf("controlled runtime prefix=%+v, want %+v", got, want)
	}
}
