package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPairedVariantsFreezeBrainScenarioOracleAndBudgets(t *testing.T) {
	authority := validPairedRunAuthority(t)
	left := VariantResult{Authority: authority, Variant: VariantRawObservation, EpisodeSealSHA256: strings.Repeat("e", 64)}
	right := VariantResult{Authority: authority, Variant: VariantFullCognition, EpisodeSealSHA256: strings.Repeat("f", 64)}
	if err := RequirePairedVariants(left, right); err != nil {
		t.Fatal(err)
	}

	changed := right
	changed.Authority.Budget.ToolOperations++
	if err := RequirePairedVariants(left, changed); err == nil {
		t.Fatal("changed tool budget was accepted as a paired variant")
	}
	changed = right
	changed.Authority.Budget.Station.MaxOutputTokens++
	if err := RequirePairedVariants(left, changed); err == nil {
		t.Fatal("changed per-call station budget was accepted as a paired variant")
	}
	changed = right
	changed.Authority.RatGeneration.Fixed.Brain.Digest = strings.Repeat("9", 64)
	if err := RequirePairedVariants(left, changed); err == nil {
		t.Fatal("changed model was accepted as a paired variant")
	}
	changed = right
	changed.Authority.OracleSHA256 = strings.Repeat("8", 64)
	if err := RequirePairedVariants(left, changed); err == nil {
		t.Fatal("changed seed/oracle authority was accepted as a paired variant")
	}
	changed = right
	changed.Authority.Runtime.PromptSHA256 = strings.Repeat("7", 64)
	if err := RequirePairedVariants(left, changed); err == nil {
		t.Fatal("changed prompt authority was accepted as a paired variant")
	}
}

func TestPairedVariantsRejectInvalidOrIdenticalAblations(t *testing.T) {
	authority := validPairedRunAuthority(t)
	left := VariantResult{Authority: authority, Variant: VariantTaskLedger, EpisodeSealSHA256: strings.Repeat("e", 64)}
	if err := RequirePairedVariants(left, left); err == nil {
		t.Fatal("identical cognition variants were compared")
	}
	invalid := left
	invalid.Variant = "transcript_if_confused"
	if err := invalid.Validate(); err == nil {
		t.Fatal("unregistered fallback variant was accepted")
	}
	invalid = left
	invalid.Authority.Budget.ContextBytes++
	if err := invalid.Validate(); err == nil {
		t.Fatal("run context differs from frozen Rat Doctrine ceiling")
	}
}

func TestGeneratedPairedAuthorityBindsExactSeedSurfaceAndCatalog(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	authority, err := fixture.PairedAuthority(
		SurfaceFilesystem, mustRatGeneration(t), 2, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if authority.Seed != fixture.Spec().Generator.Seed ||
		authority.SurfaceVersion != "filesystem.v1" ||
		authority.ActionCatalogSHA256 != fixture.PublicArtifact().World.Catalog.SHA256 {
		t.Fatalf("paired authority=%+v", authority)
	}
	changed := authority
	changed.Seed++
	left := VariantResult{Authority: authority, Variant: VariantRawObservation, EpisodeSealSHA256: strings.Repeat("e", 64)}
	right := VariantResult{Authority: changed, Variant: VariantFullCognition, EpisodeSealSHA256: strings.Repeat("f", 64)}
	if err := RequirePairedVariants(left, right); err == nil {
		t.Fatal("different procedural seeds were accepted as paired variants")
	}
}

func TestRunBudgetProducesExactProductionRuntimeBudget(t *testing.T) {
	authority := validPairedRunAuthority(t)
	got, err := authority.Budget.RuntimeBudget()
	if err != nil {
		t.Fatal(err)
	}
	if got.RemainingPolicyCalls != uint32(authority.Budget.ModelCalls) ||
		got.MaxInputBytes != authority.Budget.Station.MaxInputBytes ||
		got.MaxOutputTokens != authority.Budget.Station.MaxOutputTokens ||
		got.MaxEvidenceRefs != authority.Budget.Decision.MaxEvidenceRefs ||
		got.MaxExpectedEffectBytes != authority.Budget.Decision.MaxExpectedEffectBytes {
		t.Fatalf("runtime budget=%#v authority=%#v", got, authority.Budget)
	}
	invalid := authority.Budget
	invalid.Decision.MaxEvidenceRefs = cognition.MaxEvidenceRefs + 1
	if _, err := invalid.RuntimeBudget(); err == nil {
		t.Fatal("out-of-range decision budget was accepted")
	}
}

func validPairedRunAuthority(t *testing.T) PairedRunAuthority {
	t.Helper()
	generation := mustRatGeneration(t)
	return PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1, CaseID: "case-retrieve-1",
		Suite: SuiteRetrieve, FixtureVersion: "microgauntlets.v1",
		GeneratorVersion: "generator.v1", Seed: 7,
		Scenario: cognition.ScenarioRef{
			ID:     cognition.ScenarioID("scenario-" + strings.Repeat("a", 64)),
			SHA256: strings.Repeat("b", 64),
		},
		OracleSHA256: strings.Repeat("c", 64), SurfaceVersion: "symbolic.v1",
		ActionCatalogVersion: "grammar.v1", ActionCatalogSHA256: strings.Repeat("d", 64),
		RatGeneration: generation,
		Runtime:       transferTestFingerprint(),
		Budget: RunBudget{
			ContextBytes:    generation.Fixed.ContextCeilingBytes,
			WorkingSetBytes: 8192, RuntimeCycles: 64, ModelCalls: 16,
			EnvironmentActions: 32, ToolOperations: 64,
			Station: testStationBudget(),
			Decision: DecisionBudget{
				MaxEvidenceRefs: 16, MaxActionArguments: 8,
				MaxLedgerProposals: 8, MaxAttentionRequests: 8,
				MaxExpectedEffectBytes: 1024,
			},
		},
		Repetition: 1,
	}
}
