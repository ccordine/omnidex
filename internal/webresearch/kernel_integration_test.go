package webresearch

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

func TestProductionAcquisitionUsesRegisteredKernelContractAndAttemptBudget(t *testing.T) {
	candidate := candidateFixture("https://kernel.example/evidence", "Kernel evidence")
	acquisition := &scriptedAcquisition{discoveries: map[string]discoverOutcome{
		"kernel evidence": {report: candidateReport("kernel evidence", candidate)},
	}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_kernel", Question: "What does the evidence establish?", InitialQuery: "kernel evidence", Status: ObjectivePending,
	}, acquisition, &recordingRelevanceStation{}, &recordingSynthesisStation{}, 2_000)
	registration := machine.contracts.discovery.Registration()
	if registration.Capability() != discoveryCapability || registration.Workflow() != discoveryWorkflow || registration.Version() != "1" {
		t.Fatalf("discovery registration=%#v", registration)
	}
	fetchRegistration := machine.contracts.fetch.Registration()
	if fetchRegistration.Capability() != fetchCapability || fetchRegistration.Workflow() != fetchWorkflow || fetchRegistration.Version() != "1" {
		t.Fatalf("fetch registration=%#v", fetchRegistration)
	}
	budget, err := specialistworkflow.NewAttemptBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := machine.discover(context.Background(), budget, "kernel evidence"); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.discover(context.Background(), budget, "kernel evidence"); !errors.Is(err, specialistworkflow.ErrAttemptBudgetExhausted) {
		t.Fatalf("second discovery error=%v want attempt budget exhausted", err)
	}
	if len(acquisition.events) != 1 {
		t.Fatalf("acquisition executions=%d want exactly 1", len(acquisition.events))
	}
}
