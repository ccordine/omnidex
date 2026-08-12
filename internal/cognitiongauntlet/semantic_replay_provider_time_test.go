package cognitiongauntlet

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticProviderBootstrapTimeBindsSealedEpisodeWindow(t *testing.T) {
	state, initial, _ := semanticInitialProviderInvocationState(t)
	started := initial.RecordedAt
	state.trace.Header = queue.CognitionSealedTracePage{
		EpisodeID: initial.EpisodeID, EpisodeStartedAt: started,
		SealedAt: started.Add(time.Minute),
	}
	state.initialBootstrapTrace = nil
	record := semanticReplayRawRecord(
		queue.CognitionTraceKindProviderBrainBootstrap, 0, 1, 0,
		initial.SourceID, semanticReplayJSON(t, initial),
	)
	if _, err := state.mapProviderBrainBootstrap(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err != nil {
		t.Fatal(err)
	}

	changed := initial
	changed.RecordedAt = started.Add(time.Microsecond)
	changedRecord := semanticReplayRawRecord(
		queue.CognitionTraceKindProviderBrainBootstrap, 0, 1, 0,
		changed.SourceID, semanticReplayJSON(t, changed),
	)
	changedState, _, _ := semanticInitialProviderInvocationState(t)
	changedState.trace.Header = state.trace.Header
	changedState.initialBootstrapTrace = nil
	if _, err := changedState.mapProviderBrainBootstrap(
		changedRecord, semanticReplaySourceForRecord(t, 1, changedRecord),
	); err == nil {
		t.Fatal("initial provider bootstrap accepted a time outside exact episode start")
	}
}

func TestSemanticInitialBootstrapUsesStableBrainAndExactLiveObservation(t *testing.T) {
	state, initial, _ := semanticInitialProviderInvocationState(t)
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(state.frozenBrain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := newWitnessProviderIdentity(
		state.frozenBrain.Attestation, request.ChallengeSHA256, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	live, err := cognitionpolicy.NewAttestedBrain(
		state.frozenBrain.Ref, state.frozenBrain.Attestation,
		observed.Observation, state.frozenBrain.Host,
	)
	if err != nil || live == state.frozenBrain {
		t.Fatalf("live bootstrap fixture did not change its observation: %v", err)
	}
	initial.Brain = live
	initial.Evidence = observed.Evidence.Ref
	initial.RecordedAt = observed.Observation.ObservedAt
	state.trace.Header = queue.CognitionSealedTracePage{
		EpisodeID: initial.EpisodeID, EpisodeStartedAt: initial.RecordedAt,
		SealedAt: initial.RecordedAt.Add(time.Minute),
	}
	state.initialBootstrapTrace = nil
	record := semanticReplayRawRecord(
		queue.CognitionTraceKindProviderBrainBootstrap, 0, 1, 0,
		initial.SourceID, semanticReplayJSON(t, initial),
	)
	if _, err := state.mapProviderBrainBootstrap(
		record, semanticReplaySourceForRecord(t, 1, record),
	); err != nil {
		t.Fatalf("stable frozen Brain rejected exact live bootstrap observation: %v", err)
	}
}

func TestSemanticReplayAndFailureBootstrapTimesStayInsideSeal(t *testing.T) {
	started := time.Date(2026, time.August, 12, 1, 0, 0, 0, time.UTC)
	header := queue.CognitionSealedTracePage{
		EpisodeStartedAt: started, SealedAt: started.Add(time.Minute),
	}
	for _, source := range []queue.CognitionBrainBootstrapTraceSource{
		queue.CognitionBrainBootstrapEpisodeReplay,
		queue.CognitionBrainBootstrapActivationFailure,
	} {
		state, value, _ := semanticInitialProviderInvocationState(t)
		state.trace.Header = header
		state.trace.Header.EpisodeID = value.EpisodeID
		state.activationBootstraps = make(map[string]semanticActivationBootstrap)
		value.Source = source
		value.RecordedAt = started.Add(30 * time.Second)
		phase := 2
		if source == queue.CognitionBrainBootstrapEpisodeReplay {
			value.SourceID = "cognition_replay_bootstrap_" + traceTestDigest("replay-source")
		} else {
			value.SourceID = "cognition_provider_failure_" + traceTestDigest("failure-source")
			phase = 3
		}
		record := semanticReplayRawRecord(
			queue.CognitionTraceKindProviderBrainBootstrap, 0, phase,
			int64(value.Actor.Attempt), value.SourceID, semanticReplayJSON(t, value),
		)
		if _, err := state.mapProviderBrainBootstrap(
			record, semanticReplaySourceForRecord(t, 1, record),
		); err != nil {
			t.Fatalf("valid %s bootstrap time was rejected: %v", source, err)
		}
		for name, recorded := range map[string]time.Time{
			"before": started.Add(-time.Microsecond),
			"after":  header.SealedAt.Add(time.Microsecond),
		} {
			t.Run(string(source)+"_"+name, func(t *testing.T) {
				changedState, changed, _ := semanticInitialProviderInvocationState(t)
				changedState.trace.Header = state.trace.Header
				changedState.activationBootstraps = make(map[string]semanticActivationBootstrap)
				changed.Source, changed.SourceID, changed.RecordedAt = source, value.SourceID, recorded
				changedRecord := semanticReplayRawRecord(
					queue.CognitionTraceKindProviderBrainBootstrap, 0, phase,
					int64(changed.Actor.Attempt), changed.SourceID, semanticReplayJSON(t, changed),
				)
				if _, err := changedState.mapProviderBrainBootstrap(
					changedRecord, semanticReplaySourceForRecord(t, 1, changedRecord),
				); err == nil {
					t.Fatal("provider bootstrap accepted time outside sealed episode")
				}
			})
		}
	}
}
