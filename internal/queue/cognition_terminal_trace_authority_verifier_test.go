package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

func TestCognitionTerminalTraceAuthorityRejectsCompletionAndWorkerActorChanges(t *testing.T) {
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	revision := cognition.WorldRevision{
		EpisodeID: episode, Number: 2, SHA256: strings.Repeat("b", 64),
	}
	observation, err := cognition.NewObservation(
		"terminal-evidence", revision, "goal", "The goal is satisfied.",
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		"root-obligation",
		cognition.CompletionCheckRef{
			ID: "terminal-check", Version: "1.0.0", SHA256: strings.Repeat("c", 64),
		},
		revision, cognition.CompletionSatisfied,
		[]cognition.EvidenceRef{observation.EvidenceRef()},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, completionSHA, err := cognitionJSON(completion)
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 11, Generation: 12, StepID: 13, Attempt: 14,
		WorkerID: "worker-terminal-trace",
	}
	seal := CognitionTerminalSeal{
		EpisodeID: episode, Outcome: CognitionEpisodeCompleted,
		FinalRevision: revision, CompletionSHA256: completionSHA,
		ObligationGraphSHA256: strings.Repeat("d", 64),
		LedgerVersion:         3, WorkingSetVersion: 4, TraceSHA256: strings.Repeat("e", 64),
		AuthorityKind: cognitionTerminalAuthorityWorker,
		SealedBy: model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		},
		CreatedAt: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
	}
	if err := VerifyCognitionTerminalCompletionTraceAuthority(seal, completion); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionWorkerTerminalActorTraceAuthority(seal, actor); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*CognitionTerminalSeal){
		"completion_sha": func(value *CognitionTerminalSeal) {
			value.CompletionSHA256 = strings.Repeat("f", 64)
		},
		"completion_outcome": func(value *CognitionTerminalSeal) {
			value.Outcome = CognitionEpisodeFailed
		},
		"completion_revision": func(value *CognitionTerminalSeal) {
			value.FinalRevision.Number++
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := seal
			mutate(&changed)
			if VerifyCognitionTerminalCompletionTraceAuthority(changed, completion) == nil {
				t.Fatal("changed terminal completion authority was accepted")
			}
		})
	}

	for name, mutate := range map[string]func(*CognitionTerminalSeal, *cognition.AttemptRef){
		"authority_kind": func(value *CognitionTerminalSeal, _ *cognition.AttemptRef) {
			value.AuthorityKind = cognitionTerminalAuthorityLifecycle
		},
		"sealed_actor": func(value *CognitionTerminalSeal, _ *cognition.AttemptRef) {
			value.SealedBy.WorkerID = "worker-forged"
		},
		"expected_actor": func(_ *CognitionTerminalSeal, value *cognition.AttemptRef) {
			value.Attempt++
		},
	} {
		t.Run(name, func(t *testing.T) {
			changedSeal, changedActor := seal, actor
			mutate(&changedSeal, &changedActor)
			if VerifyCognitionWorkerTerminalActorTraceAuthority(changedSeal, changedActor) == nil {
				t.Fatal("changed worker terminal actor was accepted")
			}
		})
	}
}
