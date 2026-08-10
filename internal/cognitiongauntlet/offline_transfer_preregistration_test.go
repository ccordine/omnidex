package cognitiongauntlet

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOfflineTransferPreregistrationFreezesSortedSurfacesAndFixedAuthority(t *testing.T) {
	plan := OfflineTransferPlan{
		Suite: SuiteCombined, Seed: 73, Repetition: 2,
		Surfaces: []Surface{SurfaceFilesystem, SurfaceSymbolic},
	}
	fixed := validOfflineTransferFixedAuthority(t)
	registration, err := NewOfflineTransferPreregistration(plan, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Schema != OfflineTransferPreregistrationSchemaV1 ||
		registration.Variant != VariantFullCognition || registration.RunCount != 2 ||
		!reflect.DeepEqual(registration.Plan.Surfaces, []Surface{SurfaceSymbolic, SurfaceFilesystem}) {
		t.Fatalf("unexpected transfer registration: %#v", registration)
	}
	if registration.RegisteredAt.IsZero() || registration.Hypothesis == "" {
		t.Fatal("transfer registration omitted pre-inference authority")
	}

	changed := registration
	changed.Plan.Seed++
	if err := changed.Validate(); err == nil {
		t.Fatal("post-registration seed substitution was accepted")
	}
	changed = registration
	changed.Fixed.Budget.ModelCalls++
	if err := changed.Validate(); err == nil {
		t.Fatal("post-registration budget substitution was accepted")
	}
}

func TestOfflineTransferPlanRejectsMissingDuplicateAndUnsortedSurfaces(t *testing.T) {
	base := OfflineTransferPlan{
		Suite: SuiteCombined, Seed: 73, Repetition: 1,
		Surfaces: []Surface{SurfaceSymbolic, SurfaceFilesystem},
	}
	for name, mutate := range map[string]func(*OfflineTransferPlan){
		"one":       func(plan *OfflineTransferPlan) { plan.Surfaces = []Surface{SurfaceSymbolic} },
		"duplicate": func(plan *OfflineTransferPlan) { plan.Surfaces[1] = SurfaceSymbolic },
		"unsorted": func(plan *OfflineTransferPlan) {
			plan.Surfaces[0], plan.Surfaces[1] = plan.Surfaces[1], plan.Surfaces[0]
		},
		"resume": func(plan *OfflineTransferPlan) { plan.Suite = SuiteResume },
	} {
		t.Run(name, func(t *testing.T) {
			plan := base
			plan.Surfaces = append([]Surface{}, base.Surfaces...)
			mutate(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("invalid transfer plan was accepted")
			}
		})
	}
}

func validOfflineTransferFixedAuthority(t *testing.T) OfflineMatrixFixedAuthority {
	t.Helper()
	generation := mustRatGeneration(t)
	budget, err := NewExecutableRunBudgetV2(
		InitialMicrogauntletsV1()[4].Budget, generation.Fixed.Brain.Sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	return OfflineMatrixFixedAuthority{
		Budget: budget, RatGeneration: generation,
		RuntimeFingerprint: RuntimeFingerprint{
			ProductionSourceSHA256: generation.Runtime.SourceSHA256,
			RendererSHA256:         strings.Repeat("2", 64), RetentionPolicySHA256: strings.Repeat("3", 64),
			ObligationPolicySHA256: strings.Repeat("4", 64), PromptSHA256: strings.Repeat("5", 64),
		},
		InferenceTimeoutSeconds: 60,
		OmnidexCommit:           "0123456789abcdef0123456789abcdef01234567",
		LedgerSchemaVersion:     "ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "projection.v1",
	}
}

func TestOfflineTransferPreregistrationRejectsFutureTime(t *testing.T) {
	registration, err := NewOfflineTransferPreregistration(OfflineTransferPlan{
		Suite: SuiteRetrieve, Seed: 9, Repetition: 1,
		Surfaces: []Surface{SurfaceSymbolic, SurfaceFilesystem},
	}, validOfflineTransferFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	registration.RegisteredAt = time.Now().UTC().Add(2 * time.Minute)
	if err := registration.Validate(); err == nil {
		t.Fatal("future transfer registration was accepted")
	}
}
