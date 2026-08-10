package cognitiongauntlet

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOfflineResumePreregistrationFreezesWorkloadAndSchedules(t *testing.T) {
	registration, err := NewOfflineResumePreregistration(
		OfflineResumePlan{Seed: 15_001, Repetition: 1, Surface: SurfaceFilesystem},
		matrixFixedAuthority(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Suite != SuiteResume || registration.Workload.Suite() != SuiteCombined ||
		registration.RecoveryReservePolicyCalls != OfflineResumeRecoveryReserveCalls ||
		len(registration.Schedules) != 5 || !validDigest(registration.WorkloadSpecSHA256) {
		t.Fatalf("Resume preregistration=%+v", registration)
	}
	path := filepath.Join(t.TempDir(), "resume-preregistration.json")
	if err := SealOfflineResumePreregistration(path, registration); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOfflineResumePreregistration(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, registration) {
		t.Fatal("sealed Resume preregistration changed")
	}
}

func TestOfflineResumePreregistrationRejectsScheduleAndWorkloadSubstitution(t *testing.T) {
	registration, err := NewOfflineResumePreregistration(
		OfflineResumePlan{Seed: 15_001, Repetition: 1, Surface: SurfaceFilesystem},
		matrixFixedAuthority(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []func(*OfflineResumePreregistration){
		func(value *OfflineResumePreregistration) { value.Plan.Seed++ },
		func(value *OfflineResumePreregistration) { value.Workload.Initial.CaseID += "-changed" },
		func(value *OfflineResumePreregistration) { value.WorkloadSpecSHA256 = strings.Repeat("9", 64) },
		func(value *OfflineResumePreregistration) { value.Schedules[1].DecisionBoundaries[0]++ },
		func(value *OfflineResumePreregistration) { value.Fixed.Budget.RuntimeCycles++ },
		func(value *OfflineResumePreregistration) { value.RecoveryReservePolicyCalls++ },
	}
	for index, mutate := range tests {
		candidate := registration
		candidate.Schedules = cloneResumeSchedules(registration.Schedules)
		initial := *registration.Workload.Initial
		candidate.Workload.Initial = &initial
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("Resume preregistration mutation %d was accepted", index)
		}
	}
}
