package cognitiongauntlet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func TestOfflinePrepareDerivesReleaseRuntimeAndSamplingAuthority(t *testing.T) {
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	commit, source := strings.Repeat("a", 40), strings.Repeat("b", 64)
	provider, host := offlinePrepareTestAttestations(t)
	prepared, err := prepareOfflineExperiment(
		request, provider, host, executable, commit, source, strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := prepared.promotion
	executableSHA, err := executableSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.mode != OfflineExperimentRun || config.OmnidexCommit != commit ||
		config.RatGeneration.Runtime.SourceSHA256 != source ||
		config.RatGeneration.Runtime.MigrationsSHA256 != strings.Repeat("c", 64) ||
		config.RatGeneration.Runtime.ExecutableSHA256 != executableSHA ||
		config.RuntimeFingerprint.ProductionSourceSHA256 != source ||
		config.RatGeneration.Fixed.Brain.Sampling.MaxOutputTokens != request.Budget.Station.MaxOutputTokens ||
		config.Scenario.Budget().Schema != RunBudgetSchemaRawV2 ||
		config.Scenario.Budget().Station.MaxInputTokens !=
			request.Budget.Station.MaxInputBytes+
				config.RatGeneration.Fixed.Brain.Sampling.InputSpecialTokenReserve ||
		request.Budget.Schema != RunBudgetSchemaStructuralV1 ||
		config.RatGeneration.Fixed.Brain.SamplingSHA256 == "" {
		t.Fatalf("prepared authority=%+v", config)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"runtime_fingerprint"`, `"rat_generation"`, `"sampling"`, `"sampling_sha256"`,
		`"source_sha256"`, `"executable_sha256"`, `"omnidex_commit"`,
		`"digest"`, `"quantization"`, `"backend_version"`, `"hardware"`,
		`"migrations_directory"`,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("prepare request admits caller-authored identity %s", forbidden)
		}
	}
}

func TestOfflinePrepareSealsExactlyOneRunOrTakeoverConfiguration(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []OfflineExperimentMode{OfflineExperimentRun, OfflineExperimentTakeover} {
		t.Run(string(mode), func(t *testing.T) {
			request := offlinePrepareTestRequest(t, mode)
			provider, host := offlinePrepareTestAttestations(t)
			prepared, err := prepareOfflineExperiment(
				request, provider, host, executable,
				strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
			)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "prepared.json")
			if err := sealPreparedOfflineExperiment(path, prepared); err != nil {
				t.Fatal(err)
			}
			if mode == OfflineExperimentRun {
				if _, err := LoadOfflinePromotionConfig(path); err != nil {
					t.Fatal(err)
				}
			} else if _, err := LoadOfflineTakeoverConfig(path); err != nil {
				t.Fatal(err)
			}
			if err := sealPreparedOfflineExperiment(path, prepared); err == nil {
				t.Fatal("prepare overwrote an existing configuration")
			}
		})
	}
}

func TestOfflinePrepareRejectsUnregisteredOrAmbiguousRequests(t *testing.T) {
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	boundary := uint32(1)
	request.AfterSuccessfulActions = &boundary
	if err := request.Validate(); err == nil {
		t.Fatal("run request accepted takeover authority")
	}
	request = offlinePrepareTestRequest(t, OfflineExperimentTakeover)
	request.Variant = VariantRawObservation
	if err := request.Validate(); err == nil {
		t.Fatal("prepare accepted a variant without a serious subprocess executor")
	}
	request = offlinePrepareTestRequest(t, OfflineExperimentRun)
	request.Suite = SuiteRogue
	if err := request.Validate(); err == nil {
		t.Fatal("prepare accepted an unregistered initial suite")
	}
}

func TestOfflinePrepareCompilesRegisteredAblationWithoutDerivedCallerIdentity(t *testing.T) {
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	request.Variant = VariantRawObservation
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	provider, host := offlinePrepareTestAttestations(t)
	prepared, err := prepareOfflineExperiment(
		request, provider, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.promotion.Variant != VariantRawObservation ||
		prepared.promotion.RatGeneration.Fixed.Brain.ProviderAttestation != provider.Attestation ||
		prepared.promotion.RatGeneration.Fixed.Brain.ProviderObservation != provider.Observation ||
		prepared.promotion.RatGeneration.Fixed.Brain.HostAttestation != host {
		t.Fatalf("prepared ablation authority=%+v", prepared.promotion)
	}
}

func TestOfflinePrepareRequestFileIsBoundedAndExact(t *testing.T) {
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"duplicate":  append([]byte(`{"schema":"duplicate",`), raw[1:]...),
		"case-alias": []byte(strings.Replace(string(raw), `"mode"`, `"Mode"`, 1)),
		"oversized":  []byte(strings.Repeat("x", maxOfflineExperimentRequestBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "request.json")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadOfflineExperimentRequest(path); err == nil {
				t.Fatal("inexact or oversized prepare request was accepted")
			}
		})
	}
}

func offlinePrepareTestRequest(t *testing.T, mode OfflineExperimentMode) OfflineExperimentRequest {
	t.Helper()
	brain := mustRatGeneration(t).Fixed.Brain
	privateDirectory := t.TempDir()
	if err := os.Chmod(privateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	request := OfflineExperimentRequest{
		Schema: OfflineExperimentRequestSchemaV1, Mode: mode, Variant: VariantFullCognition,
		Suite: SuiteRetrieve, Seed: 91_001, Surface: SurfaceSymbolic,
		Budget:                  InitialMicrogauntletsV1()[0].Budget,
		DatabaseURL:             "postgres://runner:credential@127.0.0.1:5432/omnidex?sslmode=disable",
		OllamaEndpoint:          "http://127.0.0.1:11434",
		InferenceTimeoutSeconds: 90, Repetition: 1,
		PublicOutputDirectory: t.TempDir(), PrivateOutputDirectory: privateDirectory,
		Brain: OfflineBrainRequest{Model: brain.Model, NativeContextLimit: brain.NativeContextLimit},
	}
	if mode == OfflineExperimentTakeover {
		boundary := uint32(1)
		request.AfterSuccessfulActions = &boundary
	}
	return request
}

func offlinePrepareTestAttestations(
	t *testing.T,
) (llm.ObservedProviderIdentity, cognitionpolicy.HostHardwareAttestation) {
	t.Helper()
	brain := mustRatGeneration(t).Fixed.Brain
	evidence, err := witnessProviderIdentityEvidence(brain.ProviderAttestation)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := llm.NewObservedProviderIdentity(
		brain.ProviderObservation.ObservedAt, brain.ProviderAttestation,
		evidence, brain.ProviderObservation.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Observation != brain.ProviderObservation {
		t.Fatal("offline prepare fixture changed the frozen provider observation")
	}
	return observed, brain.HostAttestation
}
