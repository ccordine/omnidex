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

func TestOfflinePrepareHasNoNormalizedProviderDiscoveryFallback(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition gauntlet source directory")
	}
	path := filepath.Join(filepath.Dir(current), "offline_prepare_build.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "RequireDiscoveredProviderIdentityEvidence(") {
		t.Fatal("offline prepare does not require raw provider identity evidence")
	}
	for _, forbidden := range []string{
		"RequireDiscoveredProviderIdentity(",
		"DiscoverProviderIdentity(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("offline prepare retains normalized provider discovery %q", forbidden)
		}
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
