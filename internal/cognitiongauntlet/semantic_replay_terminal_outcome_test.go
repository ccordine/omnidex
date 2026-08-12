package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayBindsTerminalEnvironmentProgressAndPublicOutcome(t *testing.T) {
	completed := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{
			Seal: queue.CognitionTerminalSeal{Outcome: queue.CognitionEpisodeCompleted},
		}},
		worldTerminal:      true,
		worldPublicOutcome: "done",
		terminalProgress: &cognitionruntime.EpisodeProgress{
			PublicOutcome: "done",
		},
	}
	outcome := Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "done"}
	if err := completed.finishTerminalPublicOutcome(outcome); err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name   string
		mutate func(*semanticReplayState, *Outcome)
	}{
		{"nonterminal_environment", func(state *semanticReplayState, _ *Outcome) {
			state.worldTerminal = false
		}},
		{"transition_outcome", func(state *semanticReplayState, _ *Outcome) {
			state.worldPublicOutcome = "other"
		}},
		{"progress_outcome", func(state *semanticReplayState, _ *Outcome) {
			state.terminalProgress.PublicOutcome = "other"
		}},
		{"manifest_outcome", func(_ *semanticReplayState, outcome *Outcome) {
			outcome.PublicOutcome = "other"
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			state := *completed
			progress := *completed.terminalProgress
			state.terminalProgress = &progress
			changed := outcome
			mutation.mutate(&state, &changed)
			if state.finishTerminalPublicOutcome(changed) == nil {
				t.Fatal("semantic replay accepted a divergent terminal public outcome")
			}
		})
	}
}

func TestSemanticReplayBindsCanceledPublicOutcome(t *testing.T) {
	cancellation, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure, "canceled", testError("source"),
	)
	if err != nil {
		t.Fatal(err)
	}
	state := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{
			Seal: queue.CognitionTerminalSeal{Outcome: queue.CognitionEpisodeCanceled},
		}},
		cancellation: &cancellation,
	}
	outcome := Outcome{Terminal: true, PublicOutcome: "canceled", FailureCode: "policy_failure"}
	if err := state.finishTerminalPublicOutcome(outcome); err != nil {
		t.Fatal(err)
	}
	outcome.PublicOutcome = "other"
	if state.finishTerminalPublicOutcome(outcome) == nil {
		t.Fatal("semantic replay accepted a cancellation/public-manifest outcome mismatch")
	}
	outcome.PublicOutcome = "canceled"
	outcome.FailureCode = "other"
	if state.finishTerminalPublicOutcome(outcome) == nil {
		t.Fatal("semantic replay accepted a cancellation/public failure-code mismatch")
	}
}

func TestSemanticReplayBindsFailedPublicFailureCode(t *testing.T) {
	state := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{
			Seal: queue.CognitionTerminalSeal{Outcome: queue.CognitionEpisodeFailed},
		}},
		worldTerminal: true, worldPublicOutcome: "failed",
		terminalProgress: &cognitionruntime.EpisodeProgress{PublicOutcome: "failed"},
	}
	outcome := Outcome{
		Terminal: true, PublicOutcome: "failed",
		FailureCode: string(queue.CognitionEpisodeFailed),
	}
	if err := state.finishTerminalPublicOutcome(outcome); err != nil {
		t.Fatal(err)
	}
	outcome.FailureCode = "other"
	if state.finishTerminalPublicOutcome(outcome) == nil {
		t.Fatal("semantic replay accepted a failed/public failure-code mismatch")
	}
}

type testError string

func (err testError) Error() string { return string(err) }
