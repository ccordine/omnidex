package cognitiongauntlet

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayProviderInvocationPairingIsReverseComplete(t *testing.T) {
	state, trace, process := semanticInitialProviderInvocationState(t)
	if err := state.finishProviderInvocations(); err != nil {
		t.Fatal(err)
	}
	want, err := cognitionpolicy.NewProviderProcessActivation(
		process, state.evidence.identity[process.Observation.Evidence.ID], state.frozenBrain,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantAuthority, err := want.Authority()
	if err != nil {
		t.Fatal(err)
	}
	gotAuthority, err := state.providerProcessAuthority(process.ID)
	if err != nil || gotAuthority != wantAuthority {
		t.Fatalf("provider process authority=%+v error=%v", gotAuthority, err)
	}

	missingProcess, _, _ := semanticInitialProviderInvocationState(t)
	missingProcess.replayBootstraps["replay"] = trace
	if err := missingProcess.finishProviderInvocations(); err == nil {
		t.Fatal("replay bootstrap without process observation was accepted")
	}
	extraProcess, _, _ := semanticInitialProviderInvocationState(t)
	extraProcess.providerProcesses[2] = process
	if err := extraProcess.finishProviderInvocations(); err == nil {
		t.Fatal("process observation without replay bootstrap was accepted")
	}
}

func TestSemanticReplayInitialInvocationBindsStableBrainAndLiveBootstrap(t *testing.T) {
	state, initial, _ := semanticInitialProviderInvocationState(t)
	if initial.Brain == state.frozenBrain ||
		!sameFrozenBrain(initial.Brain, state.frozenBrain) {
		t.Fatal("initial invocation fixture does not distinguish live and stable Brain authority")
	}
	if err := state.finishProviderInvocations(); err != nil {
		t.Fatalf("exact live bootstrap differing only in observation was rejected: %v", err)
	}

	state.frozenBrain.Ref.Digest = traceTestDigest("different-stable-brain")
	if err := state.finishProviderInvocations(); err == nil {
		t.Fatal("initial invocation accepted a changed stable Brain component")
	}
}

func semanticInitialProviderInvocationState(t *testing.T) (
	*semanticReplayState,
	queue.CognitionBrainBootstrapTrace,
	cognitionpolicy.ProviderProcessObservation,
) {
	t.Helper()
	frozen, err := mustRatGeneration(t).Fixed.Brain.attestedBrain()
	if err != nil {
		t.Fatal(err)
	}
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(frozen.Ref)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapObserved, err := newWitnessProviderIdentity(
		frozen.Attestation, request.ChallengeSHA256, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewAttestedBrain(
		frozen.Ref, frozen.Attestation, bootstrapObserved.Observation, frozen.Host,
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapEvidence := bootstrapObserved.Evidence
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(brain, bootstrapEvidence)
	if err != nil {
		t.Fatal(err)
	}
	episode := cognition.EpisodeID("episode-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-provider-invocation",
	}
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		t.Context(), &witnessPolicyClient{model: brain.Ref.Model}, brain,
		cognition.EpisodeRef{ID: episode}, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := outcome.RequireSuccess(brain)
	if err != nil {
		t.Fatal(err)
	}
	trace := queue.CognitionBrainBootstrapTrace{
		Schema:    queue.CognitionBrainBootstrapTraceSchemaV1,
		Source:    queue.CognitionBrainBootstrapEpisodeStart,
		SourceID:  bootstrap.BootstrapEvidence.Ref.ID,
		EpisodeID: episode, Actor: actor, Brain: brain,
		Evidence:   bootstrap.BootstrapEvidence.Ref,
		RecordedAt: time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC),
	}
	state := &semanticReplayState{
		frozenBrain: frozen, initialBootstrapTrace: &trace,
		providerProcesses: map[int64]cognitionpolicy.ProviderProcessObservation{1: activation.Receipt},
		replayBootstraps:  make(map[string]queue.CognitionBrainBootstrapTrace),
		evidence: semanticReplaySupplement{identity: map[string]llm.ProviderIdentityEvidence{
			bootstrap.BootstrapEvidence.Ref.ID: bootstrap.BootstrapEvidence,
			activation.IdentityEvidence.Ref.ID: activation.IdentityEvidence,
		}},
	}
	return state, trace, activation.Receipt
}
