package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestCognitionRuntimeReplayVerifiersReuseCanonicalIDs(t *testing.T) {
	command := cognitionruntime.CompletionCommand{}
	progress := cognitionruntime.EpisodeProgress{State: cognitionruntime.ProgressCompleted}
	descriptor, err := describeCognitionRuntimeProgress(CognitionObligationSatisfy, command)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionRuntimeProgressCommandID(descriptor.ID, command, progress); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionRuntimeProgressCommandID(descriptor.ID+"x", command, progress); err == nil {
		t.Fatal("progress verifier accepted a renamed command")
	}

	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	transitionSHA := strings.Repeat("b", 64)
	transitionID := cognitionTransitionID(episode, transitionSHA)
	if err := VerifyCognitionTraceTransitionIdentity(episode, transitionID, transitionSHA); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionTraceTransitionIdentity(episode, transitionID+"x", transitionSHA); err == nil {
		t.Fatal("transition verifier accepted a renamed transition")
	}
}
