package cognitiongauntlet

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPublicAndOracleManifestsRemainSeparateAuthorities(t *testing.T) {
	public := PublicManifest{
		Schema:        PublicManifestSchemaV1,
		Suite:         SuiteRetrieve,
		Scenario:      cognition.ScenarioRef{ID: "scenario-" + cognition.ScenarioID(strings.Repeat("a", 64)), SHA256: strings.Repeat("b", 64)},
		FormatVersion: "labyrinth-public.v1", SurfaceVersion: "filesystem.v1",
		ActionCatalogVersion: "labyrinth-actions.v1", ActionCatalogSHA256: strings.Repeat("c", 64),
		Goal: "Place the requested value in the authorized target.",
		Difficulty: Difficulty{
			WorldSize: 100, RelevantArtifacts: 4, SolutionDepth: 5,
			WorkingSetBudgetBytes: 8192, ContextBudgetBytes: 16384, ToolBudget: 12,
		},
	}
	if err := public.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"seed"`, `"oracle_sha256"`, `"witness_cost"`, `"optimal_cost"`, `"relevance_labels"`, `"score"`, `"task_archetype"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("public manifest leaked %q: %s", forbidden, raw)
		}
	}

	oracle := OracleManifest{
		Schema: OracleManifestSchemaV1, ScenarioID: public.Scenario.ID,
		PublicSHA256: public.Scenario.SHA256, OracleSHA256: strings.Repeat("d", 64),
		GeneratorVersion: "solution-first.v1", Seed: 41, Quality: OracleOptimal,
		WitnessCost: 9, OptimalCost: intPointer(7), LowerBound: 7,
		TaskArchetype: "bind-delayed-evidence",
	}
	if err := oracle.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, exists := any(public).(OracleManifest); exists {
		t.Fatal("public manifest became an oracle manifest")
	}
}

func TestOracleQualityControlsEfficiencyMetricName(t *testing.T) {
	optimal, err := DecisionEfficiency(OracleOptimal, 14, 7)
	if err != nil {
		t.Fatal(err)
	}
	if optimal.Name != MetricDecisionRegret || optimal.Ratio != 2 {
		t.Fatalf("optimal metric=%+v", optimal)
	}
	witness, err := DecisionEfficiency(OracleWitnessOnly, 18, 9)
	if err != nil {
		t.Fatal(err)
	}
	if witness.Name != MetricWitnessOverhead || witness.Ratio != 2 {
		t.Fatalf("witness metric=%+v", witness)
	}
	if _, err := DecisionEfficiency(OracleWitnessOnly, 1, 0); err == nil {
		t.Fatal("zero witness cost was accepted")
	}
}

func TestDifficultyRejectsNonFiniteRatios(t *testing.T) {
	difficulty := Difficulty{
		WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
		WorkingSetBudgetBytes: 8192, ContextBudgetBytes: 16384, ToolBudget: 12,
		DistractorRatio: math.NaN(),
	}
	if err := difficulty.Validate(); err == nil {
		t.Fatal("non-finite distractor ratio was accepted")
	}
}

func intPointer(value int64) *int64 { return &value }
