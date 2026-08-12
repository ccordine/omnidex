package queue

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
)

func TestVerifyCognitionEpisodeReplayTraceInvocationBindsExactPair(t *testing.T) {
	bootstrap := freshReplayBrainBootstrap(t, cognitionTestBrainBootstrap())
	authority := model.StepAttemptAuthority{
		JobID: 41, Generation: 2, StepID: 7, Attempt: 3, WorkerID: "worker-replay-verifier",
	}
	episodeID := cognition.EpisodeID("episode-replay-verifier")
	activation := cognitionGuardProviderProcessActivationFor(
		t, t.Context(), episodeID, authority, bootstrap.AttestedBrain,
	)
	command := CognitionEpisodeStart{
		Authority: authority, EpisodeID: episodeID, BrainBootstrap: bootstrap,
		ProviderProcessActivation: activation,
	}
	projection, err := cognitionEpisodeReplayBootstrapProjectionFor(command)
	if err != nil {
		t.Fatal(err)
	}
	trace := CognitionBrainBootstrapTrace{
		Schema: CognitionBrainBootstrapTraceSchemaV1,
		Source: CognitionBrainBootstrapEpisodeReplay, SourceID: projection.ID,
		EpisodeID: episodeID,
		Actor: cognition.AttemptRef{
			JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
			Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
		},
		Brain: bootstrap.AttestedBrain, Evidence: bootstrap.BootstrapEvidence.Ref,
		RecordedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	verify := func(trace CognitionBrainBootstrapTrace,
		receipt cognitionpolicy.ProviderProcessObservation) error {
		return VerifyCognitionEpisodeReplayTraceInvocation(
			trace, bootstrap.BootstrapEvidence, receipt, activation.IdentityEvidence,
		)
	}
	if err := verify(trace, activation.Receipt); err != nil {
		t.Fatal(err)
	}
	changedID := trace
	changedID.SourceID = "cognition_replay_bootstrap_" + cognitionTestDigest("changed")
	if verify(changedID, activation.Receipt) == nil {
		t.Fatal("changed replay identity was accepted")
	}
	changedActor := trace
	changedActor.Actor.WorkerID = "worker-replay-substitution"
	if verify(changedActor, activation.Receipt) == nil {
		t.Fatal("changed replay actor was accepted")
	}
	tooEarly := activation.Receipt
	tooEarly.Observation.ObservedAt = trace.Brain.BootstrapObservation.ObservedAt.Add(-time.Microsecond)
	if verify(trace, tooEarly) == nil {
		t.Fatal("process observation before bootstrap was accepted")
	}
}
