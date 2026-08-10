package cognitiongauntlet

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOfflineScaleFamilyKeepsPrivateWorldOutOfInferenceBootstrap(t *testing.T) {
	registration := testOfflineScaleRegistration(t, 2)
	family, err := generateOfflineScaleFamily(registration)
	if err != nil {
		t.Fatal(err)
	}
	wantSizes := []int{64, 6_400, 1_000_000}
	gotSizes := make([]int, len(family.descriptor.Cases))
	for index, item := range family.descriptor.Cases {
		gotSizes[index] = item.WorldSize
	}
	if !reflect.DeepEqual(gotSizes, wantSizes) {
		t.Fatalf("world sizes=%v", gotSizes)
	}
	current := registration.Cases[len(registration.Cases)-1]
	generated, err := family.scenario(registration, current)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := generated.pairedAuthority(
		SurfaceSymbolic, registration.Fixed.RatGeneration, current.Repetition,
		registration.Fixed.RuntimeFingerprint,
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := newScenarioPublicInferenceBundle(
		generated.scenario, paired, VariantFullCognition,
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		`"seed"`, `"oracle"`, `"witness"`, `"records"`, `"corpus"`,
		`"world_size"`, `"task_archetype"`, `"private"`,
	} {
		if strings.Contains(visible, forbidden) {
			t.Fatalf("inference bootstrap exposed %s", forbidden)
		}
	}
	fixture, err := newPrivateScaleEvaluationFixture(
		registration, current, family.descriptor, generated, paired,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Family.Cases[0].WorldSize++
	if err := fixture.Validate(); err == nil {
		t.Fatal("changed private Scale family was accepted")
	}
}

func TestOfflineScaleReceiptDerivesMeasurementsAndCannotPromoteDuringEvidenceHold(t *testing.T) {
	registration := testOfflineScaleRegistration(t, 2)
	family, err := generateOfflineScaleFamily(registration)
	if err != nil {
		t.Fatal(err)
	}
	authority := testOfflineScaleAuthority(t, registration, family)
	started := registration.RegisteredAt.Add(time.Second)
	runs := make([]OfflineScaleRunReceipt, len(registration.Cases))
	for index, current := range registration.Cases {
		generated, err := family.scenario(registration, current)
		if err != nil {
			t.Fatal(err)
		}
		relevant, err := labyrinthRelevantSurfaceBytes(*generated.initial)
		if err != nil {
			t.Fatal(err)
		}
		result := OfflineScaleRunResult{
			Case: current, GeneratorVersion: family.descriptor.GeneratorVersion,
			Scenario:          generated.scenario.Ref(),
			OracleSHA256:      generated.initial.generated.PrivateOracle().OracleSHA256,
			EpisodeSealSHA256: strings.Repeat("a", 64), RelevantSurfaceBytes: relevant,
			PeakContextBytes: 12_000, ModelDecisions: 5, RetrievalRounds: 2,
			GoalSuccess: true, ValidTerminalState: true,
			CausalAdmissionComplete: true, CleanDeskQualified: true,
			CompetenceQualifiedSuccess: true,
		}
		runs[index] = OfflineScaleRunReceipt{
			Case: current, PromotionReceiptSHA256: strings.Repeat("b", 64),
			PublicAuthoritySHA256:    strings.Repeat("c", 64),
			EvaluationArtifactSHA256: strings.Repeat("d", 64),
			ScaleEvidenceSHA256:      strings.Repeat("e", 64), Result: result,
			InferenceStartedAt: started, InferenceExitedAt: started.Add(time.Second),
			EvaluatorStartedAt:   started.Add(3 * time.Second),
			EvaluatorCompletedAt: started.Add(4 * time.Second),
		}
	}
	measurements, err := deriveOfflineScaleMeasurements(registration, authority, runs)
	if err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateScaleRail(authority, measurements)
	if err != nil {
		t.Fatal(err)
	}
	registrationSHA, _ := registration.SHA256()
	receipt := OfflineScaleReceipt{
		Schema: OfflineScaleReceiptSchemaV1, PreregistrationSHA256: registrationSHA,
		Authority: authority, Runs: runs, Report: report,
		LastInferenceExitedAt:   started.Add(time.Second),
		FirstEvaluatorStartedAt: started.Add(3 * time.Second),
		CompletedAt:             started.Add(4 * time.Second),
		GateEvidenceQualified:   true, PromotionEligible: false,
	}
	if err := receipt.Validate(registration); err != nil {
		t.Fatal(err)
	}
	if receipt.PromotionEligible || !receipt.Report.Gate.Passed {
		t.Fatal("Scale promotion escaped the frozen provider/migration evidence hold")
	}
	claimed := receipt
	claimed.PromotionEligible = true
	if err := claimed.Validate(registration); err == nil {
		t.Fatal("isolated Scale receipt claimed global promotion")
	}
	changed := receipt
	changed.Runs = append([]OfflineScaleRunReceipt{}, receipt.Runs...)
	changed.Runs[0].Result.CompetenceQualifiedSuccess = false
	if err := changed.Validate(registration); err == nil {
		t.Fatal("caller-substituted Scale competence was accepted")
	}
	changed = receipt
	changed.FirstEvaluatorStartedAt = changed.LastInferenceExitedAt.Add(-time.Nanosecond)
	if err := changed.Validate(registration); err == nil {
		t.Fatal("Scale evaluation before all inference exits was accepted")
	}
	for name, mutate := range map[string]func(*OfflineScaleReceipt){
		"last inference": func(value *OfflineScaleReceipt) {
			value.LastInferenceExitedAt = value.LastInferenceExitedAt.Add(time.Second)
		},
		"first evaluator": func(value *OfflineScaleReceipt) {
			value.FirstEvaluatorStartedAt = value.FirstEvaluatorStartedAt.Add(time.Second)
		},
		"completion": func(value *OfflineScaleReceipt) {
			value.CompletedAt = value.CompletedAt.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			mutate(&changed)
			if err := changed.Validate(registration); err == nil {
				t.Fatal("Scale accepted caller-authored aggregate chronology")
			}
		})
	}
	verified := VerifiedOfflineScaleReceipt{receipt: receipt}
	copy := verified.Receipt()
	copy.Runs[0].Case.WorldSize++
	copy.Report.Measurements[0].WorldSize++
	copy.Report.Gate.Reasons = append(copy.Report.Gate.Reasons, "changed")
	if verified.receipt.Runs[0].Case.WorldSize == copy.Runs[0].Case.WorldSize ||
		verified.receipt.Report.Measurements[0].WorldSize ==
			copy.Report.Measurements[0].WorldSize ||
		len(verified.receipt.Report.Gate.Reasons) == len(copy.Report.Gate.Reasons) {
		t.Fatal("verified Scale receipt exposed mutable nested state")
	}
}

func testOfflineScaleRegistration(t *testing.T, repetitions int) OfflineScalePreregistration {
	t.Helper()
	registration, err := NewOfflineScalePreregistration(
		OfflineScalePlan{Seed: 91_731, Repetitions: repetitions},
		validOfflineTransferFixedAuthority(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func testOfflineScaleAuthority(
	t *testing.T,
	registration OfflineScalePreregistration,
	family offlineScaleGeneratedFamily,
) ScaleFamilyAuthority {
	t.Helper()
	generated, err := family.scenario(registration, registration.Cases[0])
	if err != nil {
		t.Fatal(err)
	}
	oracle := generated.initial.generated.PrivateOracle()
	authority := ScaleFamilyAuthority{
		Schema: ScaleFamilyAuthoritySchemaV1, FamilyID: family.descriptor.FamilyID,
		TaskSuite: SuiteCombined, FixtureVersion: registration.BaseWorkload.Initial.FixtureVersion,
		SurfaceVersion:        "symbolic.v1",
		ActionCatalogVersion:  family.descriptor.ActionCatalog.Version,
		ActionCatalogSHA256:   family.descriptor.ActionCatalog.SHA256,
		GoalSHA256:            family.descriptor.GoalSHA256,
		RelevantSurfaceSHA256: family.descriptor.RelevantSurfaceSHA256,
		SolutionDepth:         registration.BaseWorkload.Initial.Generator.Difficulty.SolutionDepth,
		RelevantEvidenceCount: len(oracle.RequiredEvidence),
		SemanticDecisionCount: len(oracle.Witness), Variant: VariantFullCognition,
		RatGeneration: registration.Fixed.RatGeneration, Budget: registration.Fixed.Budget,
		Runtime: registration.Fixed.RuntimeFingerprint,
	}
	if err := validateOfflineScaleAuthority(authority, registration); err != nil {
		t.Fatal(err)
	}
	return authority
}
