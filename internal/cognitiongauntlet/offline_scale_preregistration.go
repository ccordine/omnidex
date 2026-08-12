package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"
)

const OfflineScalePreregistrationSchemaV2 = "omnidex.offline-scale-preregistration.v2"

type OfflineScalePlan struct {
	Seed        uint64 `json:"seed"`
	Repetitions int    `json:"repetitions"`
}

type OfflineScaleCase struct {
	ID         string `json:"id"`
	WorldSize  int    `json:"world_size"`
	Repetition int    `json:"repetition"`
}

type OfflineScalePreregistration struct {
	Schema                 string                      `json:"schema"`
	Hypothesis             string                      `json:"hypothesis"`
	Suite                  Suite                       `json:"suite"`
	Plan                   OfflineScalePlan            `json:"plan"`
	BaseWorkload           OfflineScenarioSpec         `json:"base_workload"`
	ScaleFamilySpecSHA256  string                      `json:"scale_family_spec_sha256"`
	WorldSizes             []int                       `json:"world_sizes"`
	Surface                Surface                     `json:"surface"`
	Variant                Variant                     `json:"variant"`
	Cases                  []OfflineScaleCase          `json:"cases"`
	RunCount               int                         `json:"run_count"`
	ContextGrowthLimitPPM  int                         `json:"context_growth_limit_ppm"`
	DecisionGrowthLimitPPM int                         `json:"decision_growth_limit_ppm"`
	SuccessLossLimitBPS    int                         `json:"success_loss_limit_basis_points"`
	MinimumWorldMultiplier int                         `json:"minimum_world_multiplier"`
	Fixed                  OfflineMatrixFixedAuthority `json:"fixed"`
	RegisteredAt           time.Time                   `json:"registered_at"`
}

func NewOfflineScalePreregistration(
	plan OfflineScalePlan,
	fixed OfflineMatrixFixedAuthority,
) (OfflineScalePreregistration, error) {
	if err := plan.Validate(); err != nil {
		return OfflineScalePreregistration{}, err
	}
	if err := fixed.Validate(); err != nil {
		return OfflineScalePreregistration{}, err
	}
	registration, err := buildOfflineScalePreregistration(plan, fixed, time.Now().UTC())
	if err != nil {
		return OfflineScalePreregistration{}, err
	}
	return registration, registration.Validate()
}

func (plan OfflineScalePlan) Validate() error {
	if plan.Seed == 0 || plan.Repetitions <= 0 || plan.Repetitions > 10 {
		return fmt.Errorf("offline Scale plan authority is invalid")
	}
	return nil
}

func (registration OfflineScalePreregistration) Validate() error {
	if registration.Schema != OfflineScalePreregistrationSchemaV2 ||
		registration.Suite != SuiteScale || registration.Surface != SurfaceSymbolic ||
		registration.Variant != VariantFullCognition || registration.RegisteredAt.IsZero() ||
		registration.RegisteredAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("offline Scale preregistration authority is invalid")
	}
	if err := registration.Plan.Validate(); err != nil {
		return err
	}
	if err := registration.Fixed.Validate(); err != nil {
		return err
	}
	expected, err := buildOfflineScalePreregistration(
		registration.Plan, registration.Fixed, registration.RegisteredAt,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(registration, expected) {
		return fmt.Errorf("offline Scale preregistration differs from code-owned authority")
	}
	return nil
}

func (registration OfflineScalePreregistration) SHA256() (string, error) {
	if err := registration.Validate(); err != nil {
		return "", err
	}
	return digestJSON(registration)
}

func (registration OfflineScalePreregistration) Matches(
	plan OfflineScalePlan,
	fixed OfflineMatrixFixedAuthority,
) bool {
	return registration.Plan == plan && reflect.DeepEqual(registration.Fixed, fixed)
}

func buildOfflineScalePreregistration(
	plan OfflineScalePlan,
	fixed OfflineMatrixFixedAuthority,
	registeredAt time.Time,
) (OfflineScalePreregistration, error) {
	workload, err := ResolveOfflineScenarioSpecV1(SuiteCombined, plan.Seed, fixed.Budget)
	if err != nil {
		return OfflineScalePreregistration{}, err
	}
	workload.Initial.CaseID = fmt.Sprintf("scale-base-%d", plan.Seed)
	if err := workload.Validate(); err != nil {
		return OfflineScalePreregistration{}, err
	}
	baseSize := workload.Initial.Generator.Difficulty.WorldSize
	worldSizes := []int{baseSize, baseSize * 100, 1_000_000}
	familySHA, err := digestJSON(struct {
		Schema     string              `json:"schema"`
		Workload   OfflineScenarioSpec `json:"workload"`
		WorldSizes []int               `json:"world_sizes"`
	}{OfflineScalePreregistrationSchemaV2, workload, worldSizes})
	if err != nil {
		return OfflineScalePreregistration{}, err
	}
	cases := make([]OfflineScaleCase, 0, len(worldSizes)*plan.Repetitions)
	for _, worldSize := range worldSizes {
		for repetition := 1; repetition <= plan.Repetitions; repetition++ {
			cases = append(cases, OfflineScaleCase{
				ID:        fmt.Sprintf("scale-%d-r%d", worldSize, repetition),
				WorldSize: worldSize, Repetition: repetition,
			})
		}
	}
	return OfflineScalePreregistration{
		Schema:     OfflineScalePreregistrationSchemaV2,
		Hypothesis: "A one-hundred-times larger visible world does not materially grow model context, decisions, or success loss under one fixed cognition runtime.",
		Suite:      SuiteScale, Plan: plan, BaseWorkload: workload,
		ScaleFamilySpecSHA256: familySHA, WorldSizes: worldSizes,
		Surface: SurfaceSymbolic, Variant: VariantFullCognition,
		Cases: cases, RunCount: len(cases),
		ContextGrowthLimitPPM: 1_250_000, DecisionGrowthLimitPPM: 1_200_000,
		SuccessLossLimitBPS: 500, MinimumWorldMultiplier: 100,
		Fixed: fixed, RegisteredAt: registeredAt,
	}, nil
}
