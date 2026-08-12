package cognitiongauntlet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOfflineResumeConfigBindsPreregistrationAndDerivesRuns(t *testing.T) {
	request := validOfflineResumeRequest(t)
	baseRequest := request.baseExperiment()
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	discovery, provider, host := offlinePrepareTestEvidence(t)
	prepared, err := prepareOfflineExperiment(
		baseRequest, discovery, provider, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := resumeConfigFromBase(request, prepared.promotion)
	registration, err := NewOfflineResumePreregistration(config.Plan, config.fixedAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if err := SealOfflineResumePreregistration(config.Paths().Preregistration, registration); err != nil {
		t.Fatal(err)
	}
	config.PreregistrationSHA256, err = registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := config.derivedRunConfig(registration, registration.Schedules[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.derivedRunConfig(registration, registration.Schedules[1])
	if err != nil {
		t.Fatal(err)
	}
	if first.Scenario.Suite() != SuiteCombined || first.Variant != VariantFullCognition ||
		first.Surface != config.Plan.Surface || first.RatGeneration != second.RatGeneration ||
		first.PublicOutputDirectory == second.PublicOutputDirectory {
		t.Fatalf("derived Resume run authority changed: %+v %+v", first, second)
	}
}

func TestOfflineResumeAuthorityCanBeReloadedAfterReceiptSeals(t *testing.T) {
	request := validOfflineResumeRequest(t)
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	discovery, provider, host := offlinePrepareTestEvidence(t)
	prepared, err := prepareOfflineExperiment(
		request.baseExperiment(), discovery, provider, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := resumeConfigFromBase(request, prepared.promotion)
	registration, err := NewOfflineResumePreregistration(config.Plan, config.fixedAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if err := SealOfflineResumePreregistration(config.Paths().Preregistration, registration); err != nil {
		t.Fatal(err)
	}
	config.PreregistrationSHA256, err = registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "resume-config.json")
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Paths().Receipt, []byte("sealed-receipt-placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOfflineResumeConfig(configPath); err != nil {
		t.Fatalf("post-run auditor could not reload immutable Resume config: %v", err)
	}
	if err := config.ValidateStart(); err == nil {
		t.Fatal("Resume start accepted an existing final receipt")
	}
}

func TestOfflineResumeRequestHasNoDerivedOrScheduleAuthority(t *testing.T) {
	typeOf := reflect.TypeOf(OfflineResumeRequest{})
	for _, forbidden := range []string{
		"Schedules", "Boundaries", "Digest", "Quantization", "BackendVersion",
		"Hardware", "RuntimeFingerprint", "Measurements", "MigrationsDirectory",
	} {
		if _, exists := typeOf.FieldByName(forbidden); exists {
			t.Fatalf("Resume request exposes caller-authored %s", forbidden)
		}
	}
}

func validOfflineResumeRequest(t *testing.T) OfflineResumeRequest {
	t.Helper()
	base := offlinePrepareTestRequest(t, OfflineExperimentRun)
	return OfflineResumeRequest{
		Schema: OfflineResumeRequestSchemaV1,
		Plan:   OfflineResumePlan{Seed: 15_001, Repetition: 1, Surface: SurfaceFilesystem},
		Budget: base.Budget, DatabaseURL: base.DatabaseURL, OllamaEndpoint: base.OllamaEndpoint,
		InferenceTimeoutSeconds: base.InferenceTimeoutSeconds,
		PublicOutputDirectory:   base.PublicOutputDirectory,
		PrivateOutputDirectory:  base.PrivateOutputDirectory, Brain: base.Brain,
	}
}
