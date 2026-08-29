package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceStorageDerivesPostgreSQLForUnrelatedDurableWorkloads(t *testing.T) {
	t.Parallel()
	workloads := []assemblyline.FrozenApplicationWorkload{
		serviceStorageWorkloadFixture(t, "inventory service", "Store inventory quantities."),
		serviceStorageWorkloadFixture(t, "appointment service", "Schedule appointments."),
	}
	namespaces := make(map[string]struct{}, len(workloads))
	for _, workload := range workloads {
		lifetimes := testRequestLocalServiceStatePlan(workload)
		lifetimes.ByTask[workload.Tasks[0].ID] =
			assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
		plan, err := deriveDirectCodingServiceStoragePlan(workload, lifetimes)
		if err != nil {
			t.Fatal(err)
		}
		storage, err := plan.storageForTask(workload.Tasks[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if storage != directCodingServiceStoragePostgreSQL || !plan.RequiresPostgreSQL() ||
			plan.Namespace != "workload:"+workload.SHA256 {
			t.Fatalf("derived storage plan=%+v", plan)
		}
		namespaces[plan.Namespace] = struct{}{}
	}
	if len(namespaces) != len(workloads) {
		t.Fatalf("unrelated workloads shared a state namespace: %+v", namespaces)
	}
}

func TestServiceStorageOmitsDurableMechanicsForRequestLocalWorkload(t *testing.T) {
	t.Parallel()
	workload := serviceStorageWorkloadFixture(t, "calculator service", "Calculate one response.")
	plan, err := deriveDirectCodingServiceStoragePlan(
		workload, testRequestLocalServiceStatePlan(workload),
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresPostgreSQL() || plan.Namespace != "" ||
		plan.ByTask[workload.Tasks[0].ID] != directCodingServiceStorageRequestLocal {
		t.Fatalf("request-local storage plan=%+v", plan)
	}
}

func TestServiceStorageFailsLoudlyForInvalidAuthority(t *testing.T) {
	t.Parallel()
	workload := serviceStorageWorkloadFixture(t, "inventory service", "Store inventory quantities.")
	tests := []struct {
		name string
		edit func(*directCodingServiceStatePlan)
		want string
	}{
		{name: "missing task", edit: func(plan *directCodingServiceStatePlan) {
			delete(plan.ByTask, workload.Tasks[0].ID)
		}, want: "lifetime decisions"},
		{name: "foreign workload", edit: func(plan *directCodingServiceStatePlan) {
			plan.WorkloadSHA256 = strings.Repeat("0", len(workload.SHA256))
		}, want: "differs from frozen workload"},
		{name: "unsupported lifetime", edit: func(plan *directCodingServiceStatePlan) {
			plan.ByTask[workload.Tasks[0].ID] = assemblyline.ApplicationServiceStateLifetime("unknown")
		}, want: "unsupported lifetime"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lifetimes := testRequestLocalServiceStatePlan(workload)
			test.edit(&lifetimes)
			_, err := deriveDirectCodingServiceStoragePlan(workload, lifetimes)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("storage derivation error=%v want=%q", err, test.want)
			}
		})
	}
}

func serviceStorageWorkloadFixture(
	t *testing.T,
	product string,
	requirement string,
) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: product,
		Requirements: []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: requirement}},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	return workload
}
