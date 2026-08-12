package cognitiongauntlet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineMatrixAuthorityCanBeReloadedAfterReceiptSeals(t *testing.T) {
	base := validOfflineOutputConfig(t)
	plan := OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: []Suite{SuiteRetrieve},
		Seeds: []uint64{101}, Repetitions: 1, Surface: SurfaceFilesystem,
	}
	config := OfflineMatrixConfig{
		Schema: OfflineMatrixConfigSchemaV3, Plan: plan, Budget: base.Scenario.Budget(),
		DatabaseURL: base.DatabaseURL, OllamaEndpoint: base.OllamaEndpoint,
		InferenceTimeoutSeconds: base.InferenceTimeoutSeconds,
		PublicOutputDirectory:   base.PublicOutputDirectory,
		PrivateOutputDirectory:  base.PrivateOutputDirectory,
		RatGeneration: base.RatGeneration, PreparedBrainEvidence: base.PreparedBrainEvidence,
		RuntimeFingerprint: base.RuntimeFingerprint,
		OmnidexCommit: base.OmnidexCommit, LedgerSchemaVersion: base.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.ProjectionPolicyVersion,
	}
	registration, err := NewOfflineMatrixPreregistration(plan, config.fixedAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if err := SealOfflineMatrixPreregistration(config.Paths().Preregistration, registration); err != nil {
		t.Fatal(err)
	}
	config.PreregistrationSHA256, err = registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "matrix-config.json")
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
	if _, err := LoadOfflineMatrixConfig(configPath); err != nil {
		t.Fatalf("post-run auditor could not reload immutable config: %v", err)
	}
	if err := config.ValidateStart(); err == nil {
		t.Fatal("matrix start accepted an existing final receipt")
	}
}
