package cognitiongauntlet

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestOfflineTransferReceiptDerivesGateFromEveryPreregisteredSurface(t *testing.T) {
	registration, err := NewOfflineTransferPreregistration(OfflineTransferPlan{
		Suite: SuiteCombined, Seed: 73, Repetition: 1,
		Surfaces: []Surface{SurfaceSymbolic, SurfaceFilesystem},
	}, validOfflineTransferFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	authority := validOfflineTransferAuthority(t, registration)
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		t.Fatal(err)
	}
	started := registration.RegisteredAt.Add(time.Second)
	runs := make([]OfflineTransferRunReceipt, len(registration.Plan.Surfaces))
	episodes := make([]TransferEpisodeResult, len(runs))
	for index, surface := range registration.Plan.Surfaces {
		version, _ := surface.Version()
		seal := strings.Repeat(string(rune('a'+index)), 64)
		result := TransferEpisodeResult{
			AuthoritySHA256: authoritySHA, SurfaceVersion: version,
			Variant: VariantFullCognition, EpisodeSealSHA256: seal,
			GoalSuccess: true, CleanDeskQualified: true,
			CausalAcquisition: testCausalAcquisitionReport(seal, authority.OracleSHA256, version),
		}
		runs[index] = OfflineTransferRunReceipt{
			SurfaceVersion: version, PromotionReceiptSHA256: strings.Repeat("6", 64),
			PublicRunAuthoritySHA256: strings.Repeat("7", 64),
			EvaluationArtifactSHA256: strings.Repeat("8", 64), Result: result,
			InferenceStartedAt: started, InferenceExitedAt: started.Add(time.Second),
			EvaluatorStartedAt:   started.Add(3 * time.Second),
			EvaluatorCompletedAt: started.Add(4 * time.Second),
		}
		episodes[index] = result
	}
	report, err := EvaluateTransferRail(authority, episodes)
	if err != nil {
		t.Fatal(err)
	}
	registrationSHA, _ := registration.SHA256()
	receipt := OfflineTransferReceipt{
		Schema:                OfflineTransferReceiptSchemaV1,
		PreregistrationSHA256: registrationSHA, Authority: authority,
		Runs: runs, Report: report, LastInferenceExitedAt: started.Add(time.Second),
		FirstEvaluatorStartedAt: started.Add(3 * time.Second),
		CompletedAt:             started.Add(4 * time.Second),
		GateEvidenceQualified:   true, PromotionEligible: false,
	}
	if err := receipt.Validate(registration); err != nil {
		t.Fatal(err)
	}
	if receipt.PromotionEligible || !receipt.Report.Gate.Passed {
		t.Fatal("Transfer promotion escaped the frozen provider/migration evidence hold")
	}
	claimed := receipt
	claimed.PromotionEligible = true
	if err := claimed.Validate(registration); err == nil {
		t.Fatal("isolated Transfer receipt claimed global promotion")
	}

	changed := receipt
	changed.Runs = append([]OfflineTransferRunReceipt{}, receipt.Runs...)
	changed.Runs[0].Result.GoalSuccess = false
	if err := changed.Validate(registration); err == nil {
		t.Fatal("caller-substituted Transfer result was accepted")
	}
	changed = receipt
	changed.FirstEvaluatorStartedAt = changed.LastInferenceExitedAt.Add(-time.Nanosecond)
	if err := changed.Validate(registration); err == nil {
		t.Fatal("Transfer evaluation before all inference exits was accepted")
	}
	for name, mutate := range map[string]func(*OfflineTransferReceipt){
		"last inference": func(value *OfflineTransferReceipt) {
			value.LastInferenceExitedAt = value.LastInferenceExitedAt.Add(time.Second)
		},
		"first evaluator": func(value *OfflineTransferReceipt) {
			value.FirstEvaluatorStartedAt = value.FirstEvaluatorStartedAt.Add(time.Second)
		},
		"completion": func(value *OfflineTransferReceipt) {
			value.CompletedAt = value.CompletedAt.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			mutate(&changed)
			if err := changed.Validate(registration); err == nil {
				t.Fatal("caller-authored aggregate timestamp was accepted")
			}
		})
	}
	verified := VerifiedOfflineTransferReceipt{receipt: receipt}
	copy := verified.Receipt()
	copy.Authority.SurfaceVersions[0] = "changed"
	copy.Report.Authority.SurfaceVersions[0] = "changed"
	copy.Runs[0].Result.CausalAcquisition.AcquisitionTraceRefs[0] = "changed"
	copy.Report.Episodes[0].CausalAcquisition.AcquisitionTraceRefs[0] = "changed"
	if verified.receipt.Authority.SurfaceVersions[0] == "changed" ||
		verified.receipt.Report.Authority.SurfaceVersions[0] == "changed" ||
		verified.receipt.Runs[0].Result.CausalAcquisition.AcquisitionTraceRefs[0] == "changed" ||
		verified.receipt.Report.Episodes[0].CausalAcquisition.AcquisitionTraceRefs[0] == "changed" {
		t.Fatal("verified Transfer receipt exposed mutable nested slices")
	}
}

func validOfflineTransferAuthority(
	t *testing.T,
	registration OfflineTransferPreregistration,
) TransferAuthority {
	t.Helper()
	fixture, generator, err := offlineScenarioVersions(registration.Workload)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := sortedSurfaceVersions(registration.Plan.Surfaces)
	if err != nil {
		t.Fatal(err)
	}
	authority := TransferAuthority{
		Schema: TransferAuthoritySchemaV1, CaseID: registration.Workload.CaseID(),
		TaskSuite: registration.Plan.Suite, FixtureVersion: fixture,
		GeneratorVersion: generator, Seed: registration.Plan.Seed,
		Scenario: cognition.ScenarioRef{
			ID:     cognition.ScenarioID("scenario-" + strings.Repeat("1", 64)),
			SHA256: strings.Repeat("2", 64),
		},
		OracleSHA256: strings.Repeat("3", 64), ActionCatalogVersion: "catalog.v1",
		ActionCatalogSHA256: strings.Repeat("4", 64), SurfaceVersions: versions,
		Variant: VariantFullCognition, Repetition: registration.Plan.Repetition,
		RatGeneration: registration.Fixed.RatGeneration, Budget: registration.Fixed.Budget,
		Runtime: registration.Fixed.RuntimeFingerprint,
	}
	if err := validateOfflineTransferAuthority(authority, registration); err != nil {
		t.Fatal(err)
	}
	return authority
}
