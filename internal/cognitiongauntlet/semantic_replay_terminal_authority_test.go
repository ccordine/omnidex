package cognitiongauntlet

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayTerminalSealBindsCompletionAndWorkerActor(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	revision := cognition.WorldRevision{
		EpisodeID: episode, Number: 2, SHA256: strings.Repeat("b", 64),
	}
	observation, err := cognition.NewObservation(
		"semantic-terminal-evidence", revision, "goal", "The goal is satisfied.",
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		"root-obligation",
		cognition.CompletionCheckRef{
			ID: "semantic-terminal-check", Version: "1.0.0", SHA256: strings.Repeat("c", 64),
		},
		revision, cognition.CompletionSatisfied,
		[]cognition.EvidenceRef{observation.EvidenceRef()},
	)
	if err != nil {
		t.Fatal(err)
	}
	completionSHA, err := digestJSON(completion)
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 21, Generation: 22, StepID: 23, Attempt: 24,
		WorkerID: "worker-semantic-terminal",
	}
	seal := queue.CognitionTerminalSeal{
		EpisodeID: episode, Outcome: queue.CognitionEpisodeCompleted,
		FinalRevision: revision, CompletionSHA256: completionSHA,
		ObligationGraphSHA256: strings.Repeat("d", 64),
		LedgerVersion:         3, WorkingSetVersion: 4, TraceSHA256: strings.Repeat("e", 64),
		AuthorityKind: "worker",
		SealedBy: model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		},
		CreatedAt: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
	}
	newState := func() *semanticReplayState {
		progress := cognitionruntime.EpisodeProgress{
			Episode: cognition.EpisodeRef{ID: episode},
			State:   cognitionruntime.ProgressCompleted, Revision: revision,
			Completion: &completion,
		}
		return &semanticReplayState{
			trace:            productionTrace{Header: queue.CognitionSealedTracePage{Seal: seal}},
			terminalProgress: &progress, terminalProgressCommandID: "progress-command",
			progressCommands: map[string]semanticProgressCommandRecord{
				"progress-command": {command: cognitionruntime.CompletionCommand{
					Binding: cognitionruntime.Binding{
						Episode: cognition.EpisodeRef{ID: episode}, Attempt: actor,
					},
				}},
			},
		}
	}
	if err := newState().finishTerminalSealAuthority(); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*queue.CognitionTerminalSeal){
		"completion_sha": func(value *queue.CognitionTerminalSeal) {
			value.CompletionSHA256 = strings.Repeat("f", 64)
		},
		"authority_kind": func(value *queue.CognitionTerminalSeal) {
			value.AuthorityKind = "lifecycle"
		},
		"sealed_actor": func(value *queue.CognitionTerminalSeal) {
			value.SealedBy.WorkerID = "worker-forged"
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := newState()
			mutate(&state.trace.Header.Seal)
			if state.finishTerminalSealAuthority() == nil {
				t.Fatal("semantic replay accepted changed terminal seal authority")
			}
		})
	}
}

func TestSemanticReplayWorkerCancellationWithoutPortableActorFailsLoudly(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("1", 64))
	revision := cognition.WorldRevision{
		EpisodeID: episode, Number: 1, SHA256: strings.Repeat("2", 64),
	}
	_, graph := semanticReplayGraphFixture(t, episode)
	root := graph.Obligations[0]
	completion, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, revision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	completionSHA, err := digestJSON(completion)
	if err != nil {
		t.Fatal(err)
	}
	cancellation, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure,
		"Policy failed.", errors.New("policy failed"),
	)
	if err != nil {
		t.Fatal(err)
	}
	state := &semanticReplayState{
		trace: productionTrace{Header: queue.CognitionSealedTracePage{
			GraphVersion: 1,
			Seal: queue.CognitionTerminalSeal{
				EpisodeID: episode, Outcome: queue.CognitionEpisodeCanceled,
				FinalRevision: revision, CompletionSHA256: completionSHA,
				ObligationGraphSHA256: graph.SHA256,
				LedgerVersion:         3, WorkingSetVersion: 4,
				TraceSHA256: strings.Repeat("3", 64), AuthorityKind: "worker",
				SealedBy: model.StepAttemptAuthority{
					JobID: 31, Generation: 32, StepID: 33, Attempt: 34,
					WorkerID: "worker-unproven-cancellation",
				},
				CreatedAt: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
			},
		}},
		graphs:       map[uint64]cognition.ObligationGraphSnapshot{1: graph},
		cancellation: &cancellation,
	}
	if err := state.finishTerminalSealAuthority(); err == nil ||
		!strings.Contains(err.Error(), "lacks portable terminal actor") {
		t.Fatalf("worker cancellation terminal actor error=%v", err)
	}
}
