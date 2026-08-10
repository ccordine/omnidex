package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEpisodeSealIsDeterministicExclusiveAndTamperEvident(t *testing.T) {
	payload, err := taskstate.NewJSONObject([]byte(`{"projection_id":"context_projection_abc","prompt":"bounded"}`))
	if err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	manifest := validEpisodeManifest(generation, payload)
	path := filepath.Join(t.TempDir(), "episode.json")
	sealed, err := SealEpisode(path, manifest)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSealedEpisode(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SealSHA256 != sealed.SealSHA256 || loaded.Manifest.TraceSHA256 == "" {
		t.Fatalf("loaded seal=%+v want=%+v", loaded, sealed)
	}
	if _, err := SealEpisode(path, manifest); err == nil {
		t.Fatal("episode seal overwrote an existing report")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSealedEpisode(path); err == nil {
		t.Fatal("tampered episode seal loaded")
	}
}

func TestEvaluationIsSeparateAndBoundToSealAndOracle(t *testing.T) {
	evaluation := Evaluation{
		Schema: EvaluationSchemaV1, EpisodeSealSHA256: strings.Repeat("a", 64),
		OracleSHA256: strings.Repeat("b", 64), EvaluatorVersion: "symbolic-evaluator.v1",
		TaskArchetype: "bind-delayed-evidence",
		Quality:       OracleOptimal, GoalSuccess: true, ValidTerminalState: true,
		ActualDecisionCost: 8, ReferenceDecisionCost: 7,
	}
	if err := evaluation.Validate(); err != nil {
		t.Fatal(err)
	}
	metric, err := evaluation.EfficiencyMetric()
	if err != nil {
		t.Fatal(err)
	}
	if metric.Name != MetricDecisionRegret {
		t.Fatalf("evaluation metric=%+v", metric)
	}
	failed := evaluation
	failed.GoalSuccess = false
	failed.ValidTerminalState = false
	if err := failed.Validate(); err == nil {
		t.Fatal("failed evaluation omitted deterministic attribution")
	}
	failed.Attribution = &FailureAttribution{Class: FailureUnattributed, TraceRefs: []string{}}
	if err := failed.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodeSealRejectsChangedBrainOrContextCeiling(t *testing.T) {
	payload, err := taskstate.NewJSONObject([]byte(`{"kind":"projection"}`))
	if err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	manifest := validEpisodeManifest(generation, payload)
	manifest.Model.Digest = strings.Repeat("f", 64)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode changed the frozen model digest")
	}

	manifest = validEpisodeManifest(generation, payload)
	manifest.Resources.PeakContextBytes = int64(generation.Fixed.ContextCeilingBytes + 1)
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode exceeded the frozen context ceiling")
	}

	manifest = validEpisodeManifest(generation, payload)
	manifest.RatGeneration.Fixed.Brain.NativeContextLimit++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode tampered with the fixed rat generation")
	}

	manifest = validEpisodeManifest(generation, payload)
	manifest.Memory.CriticalEvidenceAcquired = 1
	manifest.Memory.CriticalEvidenceAtUse = 2
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode claimed more available critical evidence than it acquired")
	}
}

func TestEpisodeSealRejectsIncompleteProjectionAndTraceCounts(t *testing.T) {
	payload, err := taskstate.NewJSONObject([]byte(`{"kind":"projection"}`))
	if err != nil {
		t.Fatal(err)
	}
	manifest := validEpisodeManifest(mustRatGeneration(t), payload)
	manifest.Trace = append(manifest.Trace[:1], manifest.Trace[2:]...)
	manifest.Trace[1].Sequence = 2
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode sealed a projection that had no model call")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), payload)
	manifest.Resources.EnvironmentActions = 1
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode resource counters diverged from its trace")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), payload)
	manifest.Trace[2].Revision.Number++
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("terminal trace did not bind the final revision")
	}

	manifest = validEpisodeManifest(mustRatGeneration(t), payload)
	manifest.Trace[2].ID = manifest.Trace[1].ID
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("episode accepted duplicate trace identities")
	}
}

func mustRatGeneration(t *testing.T) RatGeneration {
	t.Helper()
	generation, err := NewRatGeneration("rat-generation-1", validFixedExperiment(), RuntimeCandidate{
		Version: "cognition-runtime.v1", SourceSHA256: strings.Repeat("c", 64),
		ExecutableSHA256: strings.Repeat("1", 64), MigrationsSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	return generation
}

func validEpisodeManifest(generation RatGeneration, payload taskstate.JSONObject) EpisodeManifest {
	episodeID := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	finalRevision := cognition.WorldRevision{EpisodeID: episodeID, Number: 1, SHA256: strings.Repeat("e", 64)}
	projectionPayload, _ := traceJSONObject(testProjectionTrace())
	callPayload, _ := traceJSONObject(testModelCallTrace())
	return EpisodeManifest{
		Schema: EpisodeManifestSchemaV1, EpisodeID: episodeID,
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
		FinalRevision: finalRevision,
		Outcome:       Outcome{Terminal: true, GoalSatisfied: true, PublicOutcome: "complete"},
		Trace: []TraceEntry{
			{Sequence: 1, Kind: TraceProjection, ID: "projection-1", Revision: &finalRevision, Payload: projectionPayload},
			{Sequence: 2, Kind: TraceModelCall, ID: "model-call-1", Revision: &finalRevision, Payload: callPayload},
			{Sequence: 3, Kind: TraceTerminal, ID: "terminal-1", Revision: &finalRevision, Payload: payload},
		},
		Resources: Resources{
			ModelCalls: 1, ModelDecisions: 1, InputTokens: 32, OutputTokens: 16,
			ContextBytes: 128, OutputBytes: 64, PeakContextBytes: 128,
		},
	}
}
