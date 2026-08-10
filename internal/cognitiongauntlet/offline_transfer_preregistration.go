package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"sort"
	"time"
)

const OfflineTransferPreregistrationSchemaV1 = "omnidex.offline-transfer-preregistration.v1"

type OfflineTransferPlan struct {
	Suite      Suite     `json:"suite"`
	Seed       uint64    `json:"seed"`
	Repetition int       `json:"repetition"`
	Surfaces   []Surface `json:"surfaces"`
}

type OfflineTransferPreregistration struct {
	Schema             string                      `json:"schema"`
	Hypothesis         string                      `json:"hypothesis"`
	Plan               OfflineTransferPlan         `json:"plan"`
	Workload           OfflineScenarioSpec         `json:"workload"`
	WorkloadSpecSHA256 string                      `json:"workload_spec_sha256"`
	Variant            Variant                     `json:"variant"`
	RunCount           int                         `json:"run_count"`
	Fixed              OfflineMatrixFixedAuthority `json:"fixed"`
	RegisteredAt       time.Time                   `json:"registered_at"`
}

func NewOfflineTransferPreregistration(
	plan OfflineTransferPlan,
	fixed OfflineMatrixFixedAuthority,
) (OfflineTransferPreregistration, error) {
	plan.Surfaces = append([]Surface{}, plan.Surfaces...)
	sort.Slice(plan.Surfaces, func(left, right int) bool {
		return transferSurfaceRank(plan.Surfaces[left]) < transferSurfaceRank(plan.Surfaces[right])
	})
	if err := plan.Validate(); err != nil {
		return OfflineTransferPreregistration{}, err
	}
	if err := fixed.Validate(); err != nil {
		return OfflineTransferPreregistration{}, err
	}
	registration, err := buildOfflineTransferPreregistration(plan, fixed, time.Now().UTC())
	if err != nil {
		return OfflineTransferPreregistration{}, err
	}
	return registration, registration.Validate()
}

func (plan OfflineTransferPlan) Validate() error {
	if offlineScenarioSuiteRank(plan.Suite) == 0 || plan.Seed == 0 ||
		plan.Repetition <= 0 || plan.Repetition > 10_000 ||
		plan.Surfaces == nil || len(plan.Surfaces) < 2 || len(plan.Surfaces) > 3 {
		return fmt.Errorf("offline Transfer plan authority is invalid")
	}
	previous := 0
	for _, surface := range plan.Surfaces {
		rank := transferSurfaceRank(surface)
		if rank == 0 || rank <= previous {
			return fmt.Errorf("offline Transfer surfaces must be registered, unique, and sorted")
		}
		previous = rank
	}
	return nil
}

func (registration OfflineTransferPreregistration) Validate() error {
	if registration.Schema != OfflineTransferPreregistrationSchemaV1 ||
		registration.Variant != VariantFullCognition ||
		registration.RegisteredAt.IsZero() ||
		registration.RegisteredAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("offline Transfer preregistration authority is invalid")
	}
	if err := registration.Plan.Validate(); err != nil {
		return err
	}
	if err := registration.Fixed.Validate(); err != nil {
		return err
	}
	expected, err := buildOfflineTransferPreregistration(
		registration.Plan, registration.Fixed, registration.RegisteredAt,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(registration, expected) {
		return fmt.Errorf("offline Transfer preregistration differs from code-owned authority")
	}
	return nil
}

func (registration OfflineTransferPreregistration) SHA256() (string, error) {
	if err := registration.Validate(); err != nil {
		return "", err
	}
	return digestJSON(registration)
}

func (registration OfflineTransferPreregistration) Matches(
	plan OfflineTransferPlan,
	fixed OfflineMatrixFixedAuthority,
) bool {
	return reflect.DeepEqual(registration.Plan, plan) && reflect.DeepEqual(registration.Fixed, fixed)
}

func buildOfflineTransferPreregistration(
	plan OfflineTransferPlan,
	fixed OfflineMatrixFixedAuthority,
	registeredAt time.Time,
) (OfflineTransferPreregistration, error) {
	workload, err := ResolveOfflineScenarioSpecV1(plan.Suite, plan.Seed, fixed.Budget)
	if err != nil {
		return OfflineTransferPreregistration{}, err
	}
	caseID := fmt.Sprintf("transfer-workload-%s-%d-r%d", plan.Suite, plan.Seed, plan.Repetition)
	if workload.Initial != nil {
		workload.Initial.CaseID = caseID
	} else {
		workload.Extended.CaseID = caseID
	}
	if err := workload.Validate(); err != nil {
		return OfflineTransferPreregistration{}, err
	}
	workloadSHA, err := digestJSON(workload)
	if err != nil {
		return OfflineTransferPreregistration{}, err
	}
	return OfflineTransferPreregistration{
		Schema:     OfflineTransferPreregistrationSchemaV1,
		Hypothesis: "One fixed cognition runtime completes the same held-out task through every preregistered surface adapter without production changes.",
		Plan: OfflineTransferPlan{
			Suite: plan.Suite, Seed: plan.Seed, Repetition: plan.Repetition,
			Surfaces: append([]Surface{}, plan.Surfaces...),
		},
		Workload: workload, WorkloadSpecSHA256: workloadSHA,
		Variant: VariantFullCognition, RunCount: len(plan.Surfaces),
		Fixed: fixed, RegisteredAt: registeredAt,
	}, nil
}

func transferSurfaceRank(surface Surface) int {
	switch surface {
	case SurfaceSymbolic:
		return 1
	case SurfaceFilesystem:
		return 2
	case SurfaceRecord:
		return 3
	default:
		return 0
	}
}
