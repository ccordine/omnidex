package cognitionruntime

import (
	"context"
	"testing"
)

func TestVerifyEpisodeProgressReusesExactPreparedAuthority(t *testing.T) {
	harness := newRuntimeHarness(t)
	harness.terminal = true
	harness.public = "The objective is complete."
	prepared, err := harness.PrepareSnapshot(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	result, err := harness.Evaluate(context.Background(), completionRequest(prepared, harness.fixture.binding))
	requireNoError(t, err)
	command := completionCommand(prepared, harness.fixture.binding, result)
	progress, err := harness.AdvanceSatisfied(context.Background(), command)
	requireNoError(t, err)
	if err := VerifyEpisodeProgress(prepared.Snapshot, command, progress); err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.PublicOutcome = "A different public outcome."
	if err := VerifyEpisodeProgress(prepared.Snapshot, changed, progress); err == nil {
		t.Fatal("progress verifier accepted a command outside the prepared authority")
	}
	changed = command
	changed.SnapshotSHA256 = runtimeDigest("another-snapshot")
	if err := VerifyEpisodeProgress(prepared.Snapshot, changed, progress); err == nil {
		t.Fatal("progress verifier accepted another snapshot identity")
	}
}
