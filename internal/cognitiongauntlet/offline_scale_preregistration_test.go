package cognitiongauntlet

import (
	"reflect"
	"testing"
)

func TestOfflineScalePreregistrationOwnsGenuineHundredAndMillionArtifactCoordinates(t *testing.T) {
	registration, err := NewOfflineScalePreregistration(
		OfflineScalePlan{Seed: 91_731, Repetitions: 2},
		validOfflineTransferFixedAuthority(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSizes := []int{64, 6_400, 1_000_000}
	if registration.Schema != OfflineScalePreregistrationSchemaV1 ||
		registration.Suite != SuiteScale || registration.Variant != VariantFullCognition ||
		registration.Surface != SurfaceSymbolic || registration.RunCount != 6 ||
		!reflect.DeepEqual(registration.WorldSizes, wantSizes) {
		t.Fatalf("unexpected Scale preregistration: %#v", registration)
	}
	if registration.BaseWorkload.Suite() != SuiteCombined ||
		registration.BaseWorkload.Seed() != registration.Plan.Seed {
		t.Fatal("Scale did not freeze one task-neutral Combined workload")
	}
	changed := registration
	changed.WorldSizes = append([]int{}, registration.WorldSizes...)
	changed.WorldSizes[1]++
	if err := changed.Validate(); err == nil {
		t.Fatal("caller-substituted Scale size was accepted")
	}
	changed = registration
	changed.Cases = append([]OfflineScaleCase{}, registration.Cases...)
	changed.Cases[1], changed.Cases[2] = changed.Cases[2], changed.Cases[1]
	if err := changed.Validate(); err == nil {
		t.Fatal("caller-reordered Scale coordinate was accepted")
	}
}

func TestOfflineScalePlanExposesNoCallerWorldOrMetricAuthority(t *testing.T) {
	typeShape := reflect.TypeOf(OfflineScalePlan{})
	for _, forbidden := range []string{
		"WorldSizes", "Measurements", "ScaleMeasurement", "ContextGrowth", "SuccessRate",
	} {
		if _, exists := typeShape.FieldByName(forbidden); exists {
			t.Fatalf("Scale plan exposes caller-authored field %s", forbidden)
		}
	}
	for _, plan := range []OfflineScalePlan{
		{}, {Seed: 1}, {Seed: 1, Repetitions: 11},
	} {
		if err := plan.Validate(); err == nil {
			t.Fatal("invalid Scale plan was accepted")
		}
	}
}
