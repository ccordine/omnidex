package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrivateOracleArtifactRequiresEvaluatorOnlyCredential(t *testing.T) {
	t.Parallel()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	paired, err := fixture.PairedAuthority(
		SurfaceSymbolic, mustRatGeneration(t), 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	private := privateEvaluationFixture{
		Schema: privateEvaluationFixtureSchemaV2,
		Scenario: OfflineScenarioSpec{
			Schema: OfflineScenarioSpecSchemaV1, Kind: OfflineScenarioInitial,
			Initial: &fixture.spec,
		},
		Surface: SurfaceSymbolic, Authority: paired,
	}
	oracle := fixture.generated.PrivateOracle()
	private.InitialOracle = &oracle
	path := filepath.Join(t.TempDir(), "private-oracle.json")
	credential := "evaluator-only-credential"
	if err := sealCredentialedJSON(path, private, credential, "private cognition evaluation fixture"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"seed"`, `"oracle"`, `"witness"`, string(private.InitialOracle.OracleSHA256),
	} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("credentialed private artifact exposed %q", forbidden)
		}
	}
	if _, err := loadPrivateEvaluationFixture(path, "wrong-credential"); err == nil {
		t.Fatal("private oracle opened without its evaluator credential")
	}
	loaded, err := loadPrivateEvaluationFixture(path, credential)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Authority.OracleSHA256 != paired.OracleSHA256 ||
		loaded.Scenario.Seed() != fixture.spec.Generator.Seed {
		t.Fatal("private evaluator artifact changed authority")
	}
}

func TestInferenceProcessAuthorityHasNoGeneratorOrEvaluatorCredential(t *testing.T) {
	t.Parallel()
	typeNames := []string{
		reflect.TypeOf(inferenceProcessConfig{}).Name(),
		reflect.TypeOf(PublicInferenceBundle{}).Name(),
	}
	for _, name := range typeNames {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "generator") || strings.Contains(lower, "privateevaluation") {
			t.Fatalf("inference authority type retained private process type %q", name)
		}
	}
	config := inferenceProcessConfig{}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(encoded)
	for _, forbidden := range []string{
		"private_oracle", "oracle_credential", "host_scenario", `"spec"`, `"seed"`, `"witness"`,
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("inference process authority contains %q: %s", forbidden, raw)
		}
	}
}
