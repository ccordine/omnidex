package cognitiongauntlet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEpisodeRecorderOwnsSequenceAndSealsOnce(t *testing.T) {
	payload, err := taskstate.NewJSONObject([]byte(`{"kind":"evidence"}`))
	if err != nil {
		t.Fatal(err)
	}
	template := validRecorderTemplate(t)
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		t.Fatal(err)
	}
	revision := cognition.WorldRevision{EpisodeID: template.EpisodeID, Number: 1, SHA256: strings.Repeat("e", 64)}
	for _, entry := range []struct {
		kind    TraceKind
		id      string
		payload taskstate.JSONObject
	}{
		{TraceProjection, "projection-1", mustTraceJSONObject(t, testProjectionTrace())},
		{TraceModelCall, "model-call-1", mustTraceJSONObject(t, testModelCallTrace())},
		{TraceTerminal, "terminal-1", payload},
	} {
		if err := recorder.Append(entry.kind, entry.id, &revision, entry.payload); err != nil {
			t.Fatal(err)
		}
	}
	seal, err := recorder.Seal(
		filepath.Join(t.TempDir(), "episode.json"), revision,
		Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "complete"},
		Resources{
			ModelCalls: 1, ModelDecisions: 1, InputTokens: 32, OutputTokens: 16,
			ContextBytes: 128, OutputBytes: 64, PeakContextBytes: 128,
		},
		MemoryMetrics{}, PlanningMetrics{}, RecoveryMetrics{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(seal.Manifest.Trace) != 3 || seal.Manifest.Trace[2].Sequence != 3 {
		t.Fatalf("sealed trace=%+v", seal.Manifest.Trace)
	}
	if err := recorder.Append(TraceFailure, "after-seal", &revision, payload); err == nil {
		t.Fatal("sealed recorder accepted another event")
	}
}

func TestEpisodeRecorderRejectsPrepopulatedRuntimeState(t *testing.T) {
	template := validRecorderTemplate(t)
	template.Trace = []TraceEntry{}
	if _, err := NewEpisodeRecorder(template); err == nil {
		t.Fatal("recorder accepted a caller-owned trace")
	}
}

func validRecorderTemplate(t *testing.T) EpisodeManifest {
	t.Helper()
	generation := mustRatGeneration(t)
	return EpisodeManifest{
		Schema:    EpisodeManifestSchemaV1,
		EpisodeID: cognition.EpisodeID("episode-" + strings.Repeat("a", 64)),
		Scenario: cognition.ScenarioRef{
			ID:     cognition.ScenarioID("scenario-" + strings.Repeat("b", 64)),
			SHA256: strings.Repeat("c", 64),
		},
		PublicRunAuthoritySHA256: strings.Repeat("f", 64), Variant: VariantFullCognition,
		RuntimeVersion: generation.Runtime.Version, LedgerSchemaVersion: "ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1", ProjectionPolicyVersion: "projection.v1",
		RatGeneration: generation, StationBudget: testStationBudget(),
		Model: ModelRecord{
			Name: generation.Fixed.Brain.Model, Digest: generation.Fixed.Brain.Digest,
			Quantization:            generation.Fixed.Brain.Quantization,
			SamplingSHA256:          generation.Fixed.Brain.SamplingSHA256,
			ContextLimit:            generation.Fixed.Brain.NativeContextLimit,
			Backend:                 generation.Fixed.Brain.Backend,
			BackendVersion:          generation.Fixed.Brain.BackendVersion,
			Hardware:                generation.Fixed.Brain.Hardware,
			HardwareAuthoritySource: generation.Fixed.Brain.HardwareAuthoritySource,
		},
	}
}
