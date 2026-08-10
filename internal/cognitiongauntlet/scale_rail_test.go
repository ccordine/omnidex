package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestScaleRailComputesPreRegisteredGateFromBoundMeasurements(t *testing.T) {
	authority := scaleTestAuthority(t)
	digest, err := digestJSON(authority)
	if err != nil {
		t.Fatal(err)
	}
	base := scaleMeasurement("scale-100", 1, 100, digest)
	large := scaleMeasurement("scale-10000", 2, 10_000, digest)
	large.MedianContextBytes = 15_000
	large.MedianModelDecisions = 11
	large.SuccessRate = .91
	report, err := EvaluateScaleRail(authority, []ScaleMeasurement{large, base})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Gate.Passed || report.GateInput.WorldMultiplier != 100 ||
		report.GateInput.ContextGrowth != 1.25 || report.GateInput.DecisionGrowth != 1.1 {
		t.Fatalf("scale report=%+v", report)
	}
	if report.ContextPerRelevantBase != 1.5 || report.ContextPerRelevantLargest != 1.875 {
		t.Fatalf("context/relevant metrics=%+v", report)
	}
}

func TestScaleRailRejectsChangedAuthorityAndRelevantSurface(t *testing.T) {
	authority := scaleTestAuthority(t)
	digest, err := digestJSON(authority)
	if err != nil {
		t.Fatal(err)
	}
	base := scaleMeasurement("scale-100", 1, 100, digest)
	large := scaleMeasurement("scale-10000", 2, 10_000, digest)
	large.FamilyAuthoritySHA256 = strings.Repeat("f", 64)
	if _, err := EvaluateScaleRail(authority, []ScaleMeasurement{base, large}); err == nil {
		t.Fatal("scale rail accepted a measurement from another frozen family")
	}
	large = scaleMeasurement("scale-10000", 2, 10_000, digest)
	large.RelevantSurfaceBytes++
	if _, err := EvaluateScaleRail(authority, []ScaleMeasurement{base, large}); err == nil {
		t.Fatal("scale rail accepted a changed relevant surface")
	}
	large = scaleMeasurement("scale-10000", 2, 10_000, digest)
	large.CausalAdmissionRate = large.SuccessRate - .01
	if _, err := EvaluateScaleRail(authority, []ScaleMeasurement{base, large}); err == nil {
		t.Fatal("scale rail accepted success not backed by causal evidence acquisition")
	}
	large = scaleMeasurement("scale-10000", 2, 10_000, digest)
	large.CleanDeskAdmissionRate = large.SuccessRate - .01
	if _, err := EvaluateScaleRail(authority, []ScaleMeasurement{base, large}); err == nil {
		t.Fatal("scale rail rewarded success after omitting critical projected evidence")
	}
}

func scaleTestAuthority(t *testing.T) ScaleFamilyAuthority {
	t.Helper()
	generation := mustRatGeneration(t)
	return ScaleFamilyAuthority{
		Schema: ScaleFamilyAuthoritySchemaV1, FamilyID: "scale-family-1",
		TaskSuite: SuiteCombined, FixtureVersion: "scale.v1", SurfaceVersion: "record-surface.v1",
		ActionCatalogVersion: "grammar.v1", ActionCatalogSHA256: strings.Repeat("a", 64),
		GoalSHA256: strings.Repeat("b", 64), RelevantSurfaceSHA256: strings.Repeat("c", 64),
		SolutionDepth: 7, RelevantEvidenceCount: 5, SemanticDecisionCount: 10,
		Variant:       VariantFullCognition,
		RatGeneration: generation,
		Budget: RunBudget{
			ContextBytes: generation.Fixed.ContextCeilingBytes, WorkingSetBytes: 8192, RuntimeCycles: 96,
			ModelCalls: 32, EnvironmentActions: 64, ToolOperations: 64,
			Station: testStationBudget(),
			Decision: DecisionBudget{
				MaxEvidenceRefs: 16, MaxActionArguments: 8,
				MaxLedgerProposals: 8, MaxAttentionRequests: 8,
				MaxExpectedEffectBytes: 1024,
			},
		},
		Runtime: transferTestFingerprint(),
	}
}

func scaleMeasurement(id string, seed uint64, world int, authoritySHA string) ScaleMeasurement {
	return ScaleMeasurement{
		CaseID: id, GeneratorVersion: "scale-generator.v1", Seed: seed,
		Scenario: cognition.ScenarioRef{
			ID:     cognition.ScenarioID("scenario-" + strings.Repeat(string(rune('a'+seed-1)), 64)),
			SHA256: strings.Repeat(string(rune('c'+seed-1)), 64),
		},
		OracleSHA256:          strings.Repeat(string(rune('e'+seed-1)), 64),
		FamilyAuthoritySHA256: authoritySHA, WorldSize: world,
		RelevantSurfaceBytes: 8_000, MedianContextBytes: 12_000,
		MedianModelDecisions: 10, SuccessRate: .95, CausalAdmissionRate: .95,
		CleanDeskAdmissionRate: .95,
		MedianRetrievalRounds:  2,
	}
}
