package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPrivateEpisodeReplayRejectsForgedWorldOutcomesAndTransition(t *testing.T) {
	successFixture, successPaired, success := runPrivateReplayEpisode(t, false)
	if !success.Manifest.Outcome.GoalSatisfied {
		t.Fatal("positive private replay fixture did not satisfy its goal")
	}
	if _, err := derivePrivateEvaluationEvidence(
		t.Context(), successFixture, successPaired.SurfaceVersion, success,
	); err != nil {
		t.Fatalf("valid private replay: %v", err)
	}

	t.Run("forged failure", func(t *testing.T) {
		manifest := success.Manifest
		manifest.Outcome.GoalSatisfied = false
		forged := resealReplayManifest(t, manifest)
		if _, err := derivePrivateEvaluationEvidence(
			t.Context(), successFixture, successPaired.SurfaceVersion, forged,
		); err == nil {
			t.Fatal("private evaluator trusted a forged failure outcome")
		}
	})

	t.Run("altered transition", func(t *testing.T) {
		manifest := success.Manifest
		manifest.Trace = append([]TraceEntry{}, success.Manifest.Trace...)
		changed := false
		for index := range manifest.Trace {
			entry := &manifest.Trace[index]
			if entry.Kind != TraceAction {
				continue
			}
			trace, err := decodeActionTrace(*entry, cognition.EpisodeRef{ID: manifest.EpisodeID})
			if err != nil {
				t.Fatal(err)
			}
			if trace.Transition == nil {
				continue
			}
			trace.Transition.PublicOutcome = "forged public transition"
			entry.Payload, err = traceJSONObject(trace)
			if err != nil {
				t.Fatal(err)
			}
			changed = true
			break
		}
		if !changed {
			t.Fatal("successful fixture contained no accepted action")
		}
		forged := resealReplayManifest(t, manifest)
		if _, err := derivePrivateEvaluationEvidence(
			t.Context(), successFixture, successPaired.SurfaceVersion, forged,
		); err == nil {
			t.Fatal("private evaluator trusted an altered action transition")
		}
	})

	t.Run("forged success", func(t *testing.T) {
		fixture, paired, partial := runPrivateReplayEpisode(t, true)
		if partial.Manifest.Outcome.GoalSatisfied {
			t.Fatal("partial fixture unexpectedly satisfied its goal")
		}
		manifest := partial.Manifest
		manifest.Outcome.GoalSatisfied = true
		forged := resealReplayManifest(t, manifest)
		if _, err := derivePrivateEvaluationEvidence(
			t.Context(), fixture, paired.SurfaceVersion, forged,
		); err == nil {
			t.Fatal("private evaluator trusted a forged success outcome")
		}
	})
}

func runPrivateReplayEpisode(
	t *testing.T,
	partial bool,
) (MicrogauntletCase, PairedRunAuthority, SealedEpisode) {
	t.Helper()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
	if err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	paired, err := fixture.PairedAuthority(
		SurfaceSymbolic, generation, 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewVariantPublicInferenceBundle(fixture, paired, VariantOracleEvidence)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 701, Generation: 2, StepID: 19, Attempt: 1, WorkerID: "private-replay-fixture",
	}
	environment, closeEnvironment, err := newBenchmarkEnvironment(
		fixture, episode, actor, SurfaceSymbolic,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeEnvironment(); err != nil {
			t.Errorf("close replay fixture: %v", err)
		}
	})
	oracle := fixture.generated.PrivateOracle()
	base := &witnessPolicyClient{
		model: generation.Fixed.Brain.Model, witness: oracle.Witness,
		evidenceUses: oracle.EvidenceUses,
	}
	var client llm.Client = base
	if partial {
		client = &terminalPolicyClient{witnessPolicyClient: base, response: `{}`}
	}
	episodeDirectory, evidenceDirectory := t.TempDir(), t.TempDir()
	result, err := RunPublicAblation(context.Background(), bundle, PublicAblationRunRequest{
		Actor: actor, Client: client, Environment: environment,
		Completion: localRuntimeCompletion{evaluator: environment.(ablationGoalEvaluator)},
		ContaminatedEvidence: &ContaminatedEvidencePacket{
			Witness: oracle.Witness, EvidenceUses: oracle.EvidenceUses,
		},
		EpisodeSealPath:     filepath.Join(episodeDirectory, "episode.json"),
		EvidenceSealPath:    filepath.Join(evidenceDirectory, "ablation-evidence.json"),
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: ablationProjectionPolicyVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture, paired, result.Episode
}

func resealReplayManifest(t *testing.T, manifest EpisodeManifest) SealedEpisode {
	t.Helper()
	sealed, err := SealEpisode(filepath.Join(t.TempDir(), "episode.json"), manifest)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
