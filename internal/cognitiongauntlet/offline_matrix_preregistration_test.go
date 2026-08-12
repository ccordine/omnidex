package cognitiongauntlet

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOfflineMatrixPreregistrationFreezesExactCodeOwnedExperiment(t *testing.T) {
	t.Parallel()
	registration, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: CompetenceEfficiencySuperiority,
		Suites: []Suite{SuiteRetrieve, SuiteRecall}, Seeds: []uint64{101, 202},
		Repetitions: 2, Surface: SurfaceFilesystem,
	}, matrixFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	if registration.SampleCount != 8 ||
		registration.RunCount != 8*len(offlineMatrixVariantOrder()) ||
		registration.ContextReductionBasisPoints != 4_500 ||
		registration.AlphaPPM != 50_000 ||
		registration.TournamentSeed != VariantRawObservation ||
		registration.TournamentSelectionPolicy != matrixTournamentSelectionPolicyV1 ||
		!reflect.DeepEqual(registration.Variants, offlineMatrixVariantOrder()) ||
		!reflect.DeepEqual(registration.TournamentRounds, offlineMatrixTournamentRounds()) ||
		!reflect.DeepEqual(registration.DiagnosticVariants,
			[]Variant{VariantTranscriptCompacted, VariantOracleEvidence}) {
		t.Fatalf("preregistration=%+v", registration)
	}
	path := filepath.Join(t.TempDir(), "preregistration.json")
	if err := SealOfflineMatrixPreregistration(path, registration); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOfflineMatrixPreregistration(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, registration) {
		t.Fatalf("loaded preregistration changed: %+v", loaded)
	}
}

func TestOfflineMatrixPreregistrationRejectsPostRunPolicySubstitution(t *testing.T) {
	t.Parallel()
	registration, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: []Suite{SuiteRetrieve},
		Seeds: []uint64{101}, Repetitions: 1, Surface: SurfaceFilesystem,
	}, matrixFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*OfflineMatrixPreregistration){
		"variant order": func(value *OfflineMatrixPreregistration) {
			value.Variants[0], value.Variants[1] = value.Variants[1], value.Variants[0]
		},
		"alpha":  func(value *OfflineMatrixPreregistration) { value.AlphaPPM++ },
		"effect": func(value *OfflineMatrixPreregistration) { value.MinimumEffectBasisPoints++ },
		"threshold": func(value *OfflineMatrixPreregistration) {
			value.ContextReductionBasisPoints = 4_000
		},
		"case": func(value *OfflineMatrixPreregistration) { value.Cases[0].Seed++ },
		"tournament": func(value *OfflineMatrixPreregistration) {
			value.TournamentRounds[0].Challenger = VariantTaskLedger
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := registration
			changed.Variants = cloneMatrixSlice(registration.Variants)
			changed.Cases = cloneMatrixSlice(registration.Cases)
			changed.TournamentRounds = cloneMatrixSlice(registration.TournamentRounds)
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatalf("preregistration accepted changed %s", name)
			}
		})
	}
}

func matrixFixedAuthority(t *testing.T) OfflineMatrixFixedAuthority {
	t.Helper()
	generation := mustRatGeneration(t)
	budget, err := NewExecutableRunBudgetV2(
		InitialMicrogauntletsV2()[0].Budget, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := RuntimeFingerprint{
		ProductionSourceSHA256: generation.Runtime.SourceSHA256,
		RendererSHA256:         strings.Repeat("2", 64), RetentionPolicySHA256: strings.Repeat("3", 64),
		ObligationPolicySHA256: strings.Repeat("4", 64), PromptSHA256: strings.Repeat("5", 64),
	}
	return OfflineMatrixFixedAuthority{
		Budget: budget, RatGeneration: generation,
		PreparedBrainEvidence: testPreparedBrainEvidenceAuthority(
			t, generation.Fixed.Brain, t.TempDir(),
		),
		RuntimeFingerprint: fingerprint, InferenceTimeoutSeconds: 60,
		OmnidexCommit: strings.Repeat("d", 40), LedgerSchemaVersion: "ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1", ProjectionPolicyVersion: "projection.v1",
	}
}

func TestOfflineMatrixPlanRejectsCallerVariantAndMeasurementFreedom(t *testing.T) {
	t.Parallel()
	typeShape := reflect.TypeOf(OfflineMatrixPlan{})
	for _, forbidden := range []string{
		"Variants", "Alpha", "MinimumEffect", "ContextReduction", "Measurements",
		"CompetenceGateInput", "ScaleMeasurement",
	} {
		if _, exists := typeShape.FieldByName(forbidden); exists {
			t.Fatalf("matrix plan exposes caller-authored derived field %s", forbidden)
		}
	}
}

func TestOfflineMatrixPreregistrationRejectsEveryExecutionAuthoritySubstitution(t *testing.T) {
	t.Parallel()
	plan := OfflineMatrixPlan{
		Policy: CompetenceSuccessSuperiority, Suites: []Suite{SuiteRetrieve},
		Seeds: []uint64{101}, Repetitions: 1, Surface: SurfaceFilesystem,
	}
	fixed := matrixFixedAuthority(t)
	registration, err := NewOfflineMatrixPreregistration(plan, fixed)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*OfflineMatrixPlan, *OfflineMatrixFixedAuthority){
		"suite":      func(plan *OfflineMatrixPlan, _ *OfflineMatrixFixedAuthority) { plan.Suites[0] = SuiteRecall },
		"seed":       func(plan *OfflineMatrixPlan, _ *OfflineMatrixFixedAuthority) { plan.Seeds[0]++ },
		"repetition": func(plan *OfflineMatrixPlan, _ *OfflineMatrixFixedAuthority) { plan.Repetitions++ },
		"surface":    func(plan *OfflineMatrixPlan, _ *OfflineMatrixFixedAuthority) { plan.Surface = SurfaceSymbolic },
		"budget": func(_ *OfflineMatrixPlan, fixed *OfflineMatrixFixedAuthority) {
			fixed.Budget.RuntimeCycles++
		},
		"brain": func(_ *OfflineMatrixPlan, fixed *OfflineMatrixFixedAuthority) {
			fixed.RatGeneration.ID += "-changed"
		},
		"runtime": func(_ *OfflineMatrixPlan, fixed *OfflineMatrixFixedAuthority) {
			fixed.RuntimeFingerprint.RendererSHA256 = strings.Repeat("9", 64)
		},
		"timeout": func(_ *OfflineMatrixPlan, fixed *OfflineMatrixFixedAuthority) {
			fixed.InferenceTimeoutSeconds++
		},
		"ledger schema": func(_ *OfflineMatrixPlan, fixed *OfflineMatrixFixedAuthority) {
			fixed.LedgerSchemaVersion += ".changed"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedPlan := plan
			changedPlan.Suites = cloneMatrixSlice(plan.Suites)
			changedPlan.Seeds = cloneMatrixSlice(plan.Seeds)
			changedFixed := fixed
			mutate(&changedPlan, &changedFixed)
			if registration.Matches(changedPlan, changedFixed) {
				t.Fatalf("preregistration accepted changed %s", name)
			}
		})
	}
}
