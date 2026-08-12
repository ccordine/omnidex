package cognitiongauntlet

import (
	"context"
	"testing"
)

func TestResumeEpisodeSemanticsIgnoreAttemptActorsButBindActions(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[4])
	if err != nil {
		t.Fatal(err)
	}
	run, err := RunOracleBaseline(
		context.Background(), fixture, oracleTestRequest(t, SurfaceSymbolic, 1),
	)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := DeriveResumeEpisodeSemantics(run.Episode)
	if err != nil {
		t.Fatal(err)
	}
	changedActor := mutateResumeActionTrace(t, run.Episode, func(action *ActionTrace) {
		action.Action.Actor.Attempt++
		action.Action.Actor.WorkerID = "replacement-worker"
	})
	afterActor, err := DeriveResumeEpisodeSemantics(changedActor)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != afterActor {
		t.Fatal("attempt-normalized Resume semantics changed with only actor authority")
	}
	changedEffect := mutateResumeActionTrace(t, run.Episode, func(action *ActionTrace) {
		if action.Transition != nil {
			action.Transition.Cost++
		}
	})
	afterEffect, err := DeriveResumeEpisodeSemantics(changedEffect)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ActionSequenceSHA256 == afterEffect.ActionSequenceSHA256 {
		t.Fatal("Resume semantics ignored a changed environment action effect")
	}
}

func mutateResumeActionTrace(
	t *testing.T,
	episode SealedEpisode,
	mutate func(*ActionTrace),
) SealedEpisode {
	t.Helper()
	manifest := episode.Manifest
	manifest.Trace = append([]TraceEntry{}, manifest.Trace...)
	for index := range manifest.Trace {
		if manifest.Trace[index].Kind != TraceAction {
			continue
		}
		var action ActionTrace
		if err := decodeTracePayload(manifest.Trace[index].Payload, &action, "test Resume action"); err != nil {
			t.Fatal(err)
		}
		mutate(&action)
		payload, err := traceJSONObject(action)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Trace[index].Payload = payload
		break
	}
	prepared, err := prepareEpisodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	result := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: prepared}
	result.SealSHA256, err = digestJSON(prepared)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
