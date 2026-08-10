package cognitiongauntlet

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestOfflineMatrixPreregistrationFreezesExactCodeOwnedExperiment(t *testing.T) {
	t.Parallel()
	registration, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: CompetenceEfficiencySuperiority,
		Suites: []Suite{SuiteRetrieve, SuiteRecall}, Seeds: []uint64{101, 202},
		Repetitions: 2, Surface: SurfaceFilesystem,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registration.SampleCount != 8 ||
		registration.RunCount != 8*len(offlineMatrixVariantOrder()) ||
		registration.ContextReductionBasisPoints != 4_500 ||
		registration.AlphaPPM != 50_000 ||
		!reflect.DeepEqual(registration.Variants, offlineMatrixVariantOrder()) {
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
	})
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
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := registration
			changed.Variants = cloneMatrixSlice(registration.Variants)
			changed.Cases = cloneMatrixSlice(registration.Cases)
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatalf("preregistration accepted changed %s", name)
			}
		})
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
