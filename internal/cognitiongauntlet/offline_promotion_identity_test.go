package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOfflinePromotionIdentityBindsReleaseVersionCommitSourceMigrationsAndExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("frozen-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	executableSHA, err := executableSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	sourceSHA := strings.Repeat("b", 64)
	config := offlineIdentityConfig(t, commit, sourceSHA, executableSHA)
	migrationsSHA := config.RatGeneration.Runtime.MigrationsSHA256
	runtimeVersion := config.RatGeneration.Runtime.Version
	got, err := validateOfflinePromotionIdentity(
		config, executable, commit, sourceSHA, migrationsSHA, runtimeVersion,
	)
	if err != nil || got != executableSHA {
		t.Fatalf("identity digest=%q error=%v", got, err)
	}

	changedCommit := config
	changedCommit.OmnidexCommit = strings.Repeat("c", 40)
	if _, err := validateOfflinePromotionIdentity(
		changedCommit, executable, commit, sourceSHA, migrationsSHA, runtimeVersion,
	); err == nil {
		t.Fatal("changed configured commit was accepted")
	}
	if _, err := validateOfflinePromotionIdentity(
		config, executable, commit, strings.Repeat("d", 64), migrationsSHA, runtimeVersion,
	); err == nil {
		t.Fatal("changed embedded source digest was accepted")
	}
	changedRuntime := config
	changedRuntime.RuntimeFingerprint.PromptSHA256 = strings.Repeat("e", 64)
	if _, err := validateOfflinePromotionIdentity(
		changedRuntime, executable, commit, sourceSHA, migrationsSHA, runtimeVersion,
	); err == nil {
		t.Fatal("caller-authored runtime fingerprint was accepted")
	}
	if _, err := validateOfflinePromotionIdentity(
		config, executable, commit, sourceSHA, strings.Repeat("d", 64), runtimeVersion,
	); err == nil {
		t.Fatal("changed embedded migration digest was accepted")
	}
	if _, err := validateOfflinePromotionIdentity(
		config, executable, commit, sourceSHA, migrationsSHA, "v9.9.9",
	); err == nil {
		t.Fatal("changed embedded runtime version was accepted")
	}
	if err := os.WriteFile(executable, []byte("replacement-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := validateOfflinePromotionIdentity(
		config, executable, commit, sourceSHA, migrationsSHA, runtimeVersion,
	); err == nil {
		t.Fatal("changed executable was accepted")
	}
}

func TestOfflinePromotionConfigRequiresReleaseMetadataShape(t *testing.T) {
	config := offlineIdentityConfig(
		t, strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64),
	)
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.OmnidexCommit = "short-commit"
	if err := config.Validate(); err == nil {
		t.Fatal("short release commit was accepted")
	}
}

func TestOfflineChildCommandCarriesNoWorldOrStandardInput(t *testing.T) {
	command, err := offlineChildCommand(
		context.Background(), "/exact/cognition-gauntlet", "infer", "/private/process.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/exact/cognition-gauntlet", "infer", "--process-config", "/private/process.json",
	}
	if !reflect.DeepEqual(command.Args, want) || command.Stdin != nil ||
		!reflect.DeepEqual(command.Env, []string{"LANG=C.UTF-8"}) {
		t.Fatalf("child command=%+v stdin=%v env=%v", command.Args, command.Stdin, command.Env)
	}
}

func TestInferenceProcessFileContainsOnlyRuntimeEndpointsAndPublicBootstrapPath(t *testing.T) {
	config := inferenceProcessConfig{
		Schema: inferenceProcessConfigSchemaV1, DatabaseURL: "postgres://restricted@db/runtime",
		DatabaseSchema: "runtime", EnvironmentURL: "http://127.0.0.1:4123",
		EnvironmentToken: "transport-token", OllamaEndpoint: "http://127.0.0.1:11434",
		TimeoutSeconds: 60, PublicBundlePath: "/public/bootstrap.json",
		EpisodePath: "/public/episode.json", ExecutableSHA256: strings.Repeat("a", 64),
		SourceSHA256: strings.Repeat("b", 64), OmnidexCommit: strings.Repeat("c", 40),
		LedgerSchemaVersion: "ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "projection.v1", Control: terminalInferenceControl(),
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"public_world"`, `"descriptor"`, `"entities"`, `"predicate_schemas"`,
		`"initial_facts"`, `"suite"`, `"difficulty"`, `"records"`, `"corpus"`,
		`"seed"`, `"oracle"`, `"witness"`, `"task_archetype"`, `"generator"`,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("inference process file contains forbidden authority %s: %s", forbidden, raw)
		}
	}
}

func offlineIdentityConfig(
	t *testing.T,
	commit string,
	sourceSHA string,
	executableSHA string,
) OfflinePromotionConfig {
	t.Helper()
	generation := mustRatGeneration(t)
	generation.Runtime.SourceSHA256 = sourceSHA
	generation.Runtime.ExecutableSHA256 = executableSHA
	runtime, err := currentRuntimeFingerprint(sourceSHA)
	if err != nil {
		t.Fatal(err)
	}
	privateDirectory := t.TempDir()
	if err := os.Chmod(privateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	return OfflinePromotionConfig{
		Schema:                  OfflinePromotionConfigSchemaV1,
		DatabaseURL:             "postgres://runner:credential@127.0.0.1:5432/omnidex?sslmode=disable",
		OllamaEndpoint:          "http://127.0.0.1:11434",
		InferenceTimeoutSeconds: 60, Spec: InitialMicrogauntletsV1()[0],
		Variant: VariantFullCognition, Surface: SurfaceSymbolic,
		RatGeneration: generation, RuntimeFingerprint: runtime,
		Repetition: 1, PublicOutputDirectory: t.TempDir(), PrivateOutputDirectory: privateDirectory,
		OmnidexCommit: commit, LedgerSchemaVersion: "task-ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1", ProjectionPolicyVersion: "projection.v1",
	}
}
