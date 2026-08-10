package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEvaluationSealsOnlyAfterExactEpisodeAndOracleBinding(t *testing.T) {
	payload, err := taskstate.NewJSONObject([]byte(`{"kind":"terminal"}`))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := prepareEpisodeManifest(validEpisodeManifest(mustRatGeneration(t), payload))
	if err != nil {
		t.Fatal(err)
	}
	episode := SealedEpisode{Schema: EpisodeSealSchemaV1, Manifest: manifest}
	episode.SealSHA256, err = digestJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, oracle := validManifestPair()
	oracle.ScenarioID = manifest.Scenario.ID
	oracle.PublicSHA256 = manifest.Scenario.SHA256
	cleanDesk, err := EvaluateCleanDesk(episode, oracle, *testProjectionRelevance(episode, oracle))
	if err != nil {
		t.Fatal(err)
	}
	evaluation := Evaluation{
		Schema: EvaluationSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256, Seed: oracle.Seed, EvaluatorVersion: "evaluator.v1",
		TaskArchetype: oracle.TaskArchetype, Quality: oracle.Quality,
		GoalSuccess: true, ValidTerminalState: true,
		ActualDecisionCost: 7, ReferenceDecisionCost: *oracle.OptimalCost,
		CleanDesk: &cleanDesk,
	}
	path := filepath.Join(t.TempDir(), "evaluation.json")
	if err := SealEvaluation(path, evaluation, episode, oracle); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadEvaluation(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EpisodeSealSHA256 != episode.SealSHA256 {
		t.Fatalf("loaded evaluation=%+v", loaded)
	}
	_, artifactSHA256, err := LoadEvaluationArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	receipt := validEvaluationArtifactReceipt(
		t, evaluation.OracleSHA256, artifactSHA256, manifest.Scenario,
	)
	if _, err := receipt.VerifyEvaluationArtifact(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string][]byte{
		"score":      []byte(strings.Replace(string(raw), `"actual_decision_cost":7`, `"actual_decision_cost":8`, 1)),
		"clean desk": []byte(strings.Replace(string(raw), `"clean_desk":{`, `"clean_desk":{"forged":true,`, 1)),
		"causal":     appendTopLevelEvaluationField(raw, `"causal_acquisition":{}`),
		"failure":    appendTopLevelEvaluationField(raw, `"attribution":{}`),
	}
	for name, mutated := range mutations {
		t.Run("receipt rejects changed "+name, func(t *testing.T) {
			mutatedPath := filepath.Join(t.TempDir(), "evaluation.json")
			if err := os.WriteFile(mutatedPath, mutated, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := receipt.VerifyEvaluationArtifact(mutatedPath); err == nil {
				t.Fatalf("receipt accepted changed %s artifact with the same oracle identity", name)
			}
		})
	}
	if err := SealEvaluation(path, evaluation, episode, oracle); err == nil {
		t.Fatal("evaluation was overwritten")
	}

	changed := evaluation
	changed.OracleSHA256 = strings.Repeat("f", 64)
	if err := ValidateEvaluationAuthority(changed, episode, oracle); err == nil {
		t.Fatal("evaluation attached a different private oracle")
	}
	changed = evaluation
	changed.ReferenceDecisionCost++
	if err := ValidateEvaluationAuthority(changed, episode, oracle); err == nil {
		t.Fatal("evaluation changed the optimal reference cost")
	}
}

func validEvaluationArtifactReceipt(
	t *testing.T,
	oracleSHA256 string,
	artifactSHA256 string,
	scenario cognition.ScenarioRef,
) OfflinePromotionReceipt {
	t.Helper()
	base := time.Now().UTC()
	return OfflinePromotionReceipt{
		Schema:                   OfflinePromotionReceiptSchemaV1,
		PublicRunAuthoritySHA256: strings.Repeat("1", 64),
		EpisodeSealSHA256:        strings.Repeat("2", 64), EvaluationOracleSHA256: oracleSHA256,
		EvaluationArtifactSHA256: artifactSHA256, ExecutableSHA256: strings.Repeat("3", 64),
		SourceSHA256: strings.Repeat("4", 64), MigrationsSHA256: strings.Repeat("5", 64),
		RuntimeVersion: "runtime.v1", OmnidexCommit: strings.Repeat("6", 40),
		DatabaseSchema: "runtime-schema", GeneratorPID: 101, GeneratorExitedAt: base,
		Host: OfflineHostReceipt{
			Schema: offlineHostReceiptSchemaV1, PID: 102, Role: "restricted-host", Scenario: scenario,
			ConfigSHA256: strings.Repeat("7", 64), ReadySHA256: strings.Repeat("8", 64),
			StartedAt: base.Add(time.Second), ExitedAt: base.Add(3 * time.Second),
		},
		InferencePID: 103, InferenceStartedAt: base.Add(1500 * time.Millisecond),
		InferenceExitedAt: base.Add(2 * time.Second),
		EvaluatorPID:      104, EvaluatorStartedAt: base.Add(4 * time.Second),
		CompletedAt: base.Add(5 * time.Second),
	}
}

func appendTopLevelEvaluationField(raw []byte, field string) []byte {
	trimmed := strings.TrimSuffix(string(raw), "\n")
	trimmed = strings.TrimSuffix(trimmed, "}")
	return []byte(trimmed + "," + field + "}\n")
}
