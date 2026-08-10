package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"
)

const (
	OfflineResumePreregistrationSchemaV2 = "omnidex.offline-resume-preregistration.v2"
	OfflineResumeRecoveryReserveCalls    = 1
)

type OfflineResumePlan struct {
	Seed       uint64  `json:"seed"`
	Repetition int     `json:"repetition"`
	Surface    Surface `json:"surface"`
}

type OfflineResumePreregistration struct {
	Schema                     string                      `json:"schema"`
	Suite                      Suite                       `json:"suite"`
	Plan                       OfflineResumePlan           `json:"plan"`
	Workload                   OfflineScenarioSpec         `json:"workload"`
	WorkloadSpecSHA256         string                      `json:"workload_spec_sha256"`
	SchedulePolicy             string                      `json:"schedule_policy"`
	RecoveryReservePolicyCalls int                         `json:"recovery_reserve_policy_calls"`
	Schedules                  []OfflineResumeSchedule     `json:"schedules"`
	Fixed                      OfflineMatrixFixedAuthority `json:"fixed"`
	RegisteredAt               time.Time                   `json:"registered_at"`
}

func NewOfflineResumePreregistration(
	plan OfflineResumePlan,
	fixed OfflineMatrixFixedAuthority,
) (OfflineResumePreregistration, error) {
	if err := plan.Validate(); err != nil {
		return OfflineResumePreregistration{}, err
	}
	if err := fixed.Validate(); err != nil {
		return OfflineResumePreregistration{}, err
	}
	workload, err := resumeWorkloadSpec(plan, fixed.Budget)
	if err != nil {
		return OfflineResumePreregistration{}, err
	}
	workloadSHA, err := digestJSON(workload)
	if err != nil {
		return OfflineResumePreregistration{}, err
	}
	depth := workload.Initial.Generator.Difficulty.SolutionDepth
	schedules, err := BuildOfflineResumeSchedules(plan.Seed, depth, fixed.Budget.ModelCalls)
	if err != nil {
		return OfflineResumePreregistration{}, err
	}
	registration := OfflineResumePreregistration{
		Schema: OfflineResumePreregistrationSchemaV2, Suite: SuiteResume,
		Plan: plan, Workload: workload, WorkloadSpecSHA256: workloadSHA,
		SchedulePolicy:             OfflineResumeSchedulePolicyV1,
		RecoveryReservePolicyCalls: OfflineResumeRecoveryReserveCalls,
		Schedules:                  schedules,
		Fixed:                      fixed, RegisteredAt: time.Now().UTC(),
	}
	return registration, registration.Validate()
}

func (plan OfflineResumePlan) Validate() error {
	if plan.Seed == 0 || plan.Repetition <= 0 || plan.Repetition > 10_000 {
		return fmt.Errorf("offline Resume plan seed or repetition is invalid")
	}
	if _, err := plan.Surface.Version(); err != nil {
		return err
	}
	return nil
}

func (registration OfflineResumePreregistration) Validate() error {
	if registration.Schema != OfflineResumePreregistrationSchemaV2 ||
		registration.Suite != SuiteResume || registration.SchedulePolicy != OfflineResumeSchedulePolicyV1 ||
		registration.RecoveryReservePolicyCalls != OfflineResumeRecoveryReserveCalls ||
		registration.RegisteredAt.IsZero() ||
		registration.RegisteredAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("offline Resume preregistration authority is invalid")
	}
	if err := registration.Plan.Validate(); err != nil {
		return err
	}
	if err := registration.Fixed.Validate(); err != nil {
		return err
	}
	wantWorkload, err := resumeWorkloadSpec(registration.Plan, registration.Fixed.Budget)
	if err != nil {
		return err
	}
	wantSHA, err := digestJSON(wantWorkload)
	if err != nil {
		return err
	}
	wantSchedules, err := BuildOfflineResumeSchedules(
		registration.Plan.Seed,
		wantWorkload.Initial.Generator.Difficulty.SolutionDepth,
		registration.Fixed.Budget.ModelCalls,
	)
	if err != nil {
		return err
	}
	if registration.Fixed.Budget.ModelCalls <
		wantWorkload.Initial.Generator.Difficulty.SolutionDepth+
			registration.RecoveryReservePolicyCalls {
		return fmt.Errorf("offline Resume lacks its preregistered policy recovery reserve")
	}
	if registration.WorkloadSpecSHA256 != wantSHA ||
		!reflect.DeepEqual(registration.Workload, wantWorkload) ||
		!reflect.DeepEqual(registration.Schedules, wantSchedules) {
		return fmt.Errorf("offline Resume preregistration differs from code-owned authority")
	}
	return nil
}

func (registration OfflineResumePreregistration) SHA256() (string, error) {
	if err := registration.Validate(); err != nil {
		return "", err
	}
	return digestJSON(registration)
}

func (registration OfflineResumePreregistration) Matches(
	plan OfflineResumePlan,
	fixed OfflineMatrixFixedAuthority,
) bool {
	return reflect.DeepEqual(registration.Plan, plan) && reflect.DeepEqual(registration.Fixed, fixed)
}

func resumeWorkloadSpec(
	plan OfflineResumePlan,
	budget RunBudget,
) (OfflineScenarioSpec, error) {
	workload, err := ResolveOfflineScenarioSpecV1(SuiteCombined, plan.Seed, budget)
	if err != nil {
		return OfflineScenarioSpec{}, err
	}
	workload.Initial.CaseID = fmt.Sprintf("resume-workload-%d-r%d", plan.Seed, plan.Repetition)
	if err := workload.Validate(); err != nil {
		return OfflineScenarioSpec{}, err
	}
	return workload, nil
}

func cloneResumeSchedules(values []OfflineResumeSchedule) []OfflineResumeSchedule {
	if values == nil {
		return nil
	}
	result := make([]OfflineResumeSchedule, len(values))
	for index, value := range values {
		value.DecisionBoundaries = append([]uint32{}, value.DecisionBoundaries...)
		result[index] = value
	}
	return result
}
