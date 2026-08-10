package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromotionParentAndInferenceSourcesCannotLoadPrivateOracleAuthority(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition gauntlet source directory")
	}
	directory := filepath.Dir(current)
	parentFiles := []string{
		"offline_promotion.go", "offline_takeover.go", "offline_promotion_host.go",
		"offline_takeover_evaluate.go",
	}
	for _, name := range parentFiles {
		assertSourceOmits(t, filepath.Join(directory, name), []string{
			"GenerateMicrogauntlet(", ".PrivateOracle()", ".oracleManifest()",
			"MicrogauntletCase", "loadPrivateEvaluationFixture(",
			"loadPrivateHostScenario(", "labyrinth.Scenario",
		})
	}
	for _, name := range []string{
		"offline_inference_process.go", "offline_inference_execute.go",
		"public_full_cognition.go", "public_full_cognition_prepare.go",
	} {
		assertSourceOmits(t, filepath.Join(directory, name), []string{
			"PrivateOracle", "privateEvaluationFixture", "GenerateMicrogauntlet(",
			"HostScenario", "SealedEnvironmentScenario", "GeneratorConfig",
		})
	}
}

func assertSourceOmits(t *testing.T, path string, forbidden []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range forbidden {
		if strings.Contains(string(raw), value) {
			t.Fatalf("%s contains forbidden promotion authority %q", filepath.Base(path), value)
		}
	}
}
