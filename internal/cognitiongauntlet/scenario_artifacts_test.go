package cognitiongauntlet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPublicAndOracleArtifactsAreSealedAndLoadedIndependently(t *testing.T) {
	public, oracle := validManifestPair()
	root := t.TempDir()
	publicPath := filepath.Join(root, "public", "scenario.json")
	oraclePath := filepath.Join(root, "private", "oracle.json")
	if err := os.Mkdir(filepath.Dir(publicPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(oraclePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SealPublicManifest(publicPath, public); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oraclePath); !os.IsNotExist(err) {
		t.Fatal("sealing the public scenario also created private oracle storage")
	}
	if err := SealOracleManifest(oraclePath, oracle); err != nil {
		t.Fatal(err)
	}
	loadedPublic, err := LoadPublicManifest(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	loadedOracle, err := LoadOracleManifest(oraclePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifestPair(loadedPublic, loadedOracle); err != nil {
		t.Fatal(err)
	}
	if err := SealPublicManifest(publicPath, public); err == nil {
		t.Fatal("public scenario was overwritten")
	}
}

func TestManifestPairRejectsCrossScenarioOracle(t *testing.T) {
	public, oracle := validManifestPair()
	oracle.ScenarioID = cognition.ScenarioID("scenario-" + strings.Repeat("f", 64))
	if err := ValidateManifestPair(public, oracle); err == nil {
		t.Fatal("private oracle was attached to a different public scenario")
	}
}

func TestManifestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public.json")
	if err := os.WriteFile(path, []byte(`{"schema":"x","hidden_state":"leak"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPublicManifest(path); err == nil {
		t.Fatal("public scenario accepted an unknown hidden-state field")
	}
}

func TestManifestLoadRejectsDuplicateAndNestedCaseAliasAuthority(t *testing.T) {
	public, _ := validManifestPair()
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"duplicate": func(value string) string {
			return strings.Replace(value, `"schema":`, `"schema":"duplicate","schema":`, 1)
		},
		"nested case alias": func(value string) string {
			return strings.Replace(value, `"id":`, `"ID":`, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "public.json")
			if err := os.WriteFile(path, []byte(mutate(string(raw))), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPublicManifest(path); err == nil {
				t.Fatalf("public scenario accepted %s authority", name)
			}
		})
	}
}

func validManifestPair() (PublicManifest, OracleManifest) {
	public := PublicManifest{
		Schema: PublicManifestSchemaV1, Suite: SuiteCombined,
		Scenario: cognition.ScenarioRef{
			ID:     cognition.ScenarioID("scenario-" + strings.Repeat("a", 64)),
			SHA256: strings.Repeat("b", 64),
		},
		FormatVersion: "labyrinth-public.v1", SurfaceVersion: "filesystem.v1",
		ActionCatalogVersion: "actions.v1", ActionCatalogSHA256: strings.Repeat("c", 64),
		Goal: "Satisfy the registered public goal.",
		Difficulty: Difficulty{
			WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
			WorkingSetBudgetBytes: 8192, ContextBudgetBytes: 16384, ToolBudget: 12,
		},
	}
	oracle := OracleManifest{
		Schema: OracleManifestSchemaV1, ScenarioID: public.Scenario.ID,
		PublicSHA256: public.Scenario.SHA256, OracleSHA256: strings.Repeat("d", 64),
		GeneratorVersion: "solution-first.v1", Seed: 7, Quality: OracleOptimal,
		WitnessCost: 6, OptimalCost: intPointer(6), LowerBound: 6,
		TaskArchetype: "bounded-prerequisite-mutation",
	}
	return public, oracle
}
