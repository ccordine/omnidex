package worker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDurableStateInterfaceDoesNotFlowBackwardOrTransitivelyToProviders(t *testing.T) {
	t.Parallel()
	workload := directedStateWorkload(t,
		"Retain the accepted value between requests.",
		"Supply a request-local value used by the retained value.",
		"Supply a request-local value used by the direct provider.",
	)
	capabilities := directCodingCapabilityGraph{
		workload.Tasks[0].RequirementID: {{
			RequirementID: workload.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
			Purpose:       workload.Tasks[1].RequirementQuote,
		}},
		workload.Tasks[1].RequirementID: {{
			RequirementID: workload.Tasks[2].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(3),
			Purpose:       workload.Tasks[2].RequirementQuote,
		}},
		workload.Tasks[2].RequirementID: nil,
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan = bindTestServiceStateInterfaces(
		t, workload, capabilities, plan, testStringServiceStateField("retained"),
	)

	if _, exists, err := plan.interfaceForTask(workload.Tasks[1].ID); err != nil || exists {
		t.Fatalf("direct provider received consumer state interface: exists=%t error=%v", exists, err)
	}
	if _, exists, err := plan.interfaceForTask(workload.Tasks[2].ID); err != nil || exists {
		t.Fatalf("transitive provider received consumer state interface: exists=%t error=%v", exists, err)
	}
	if binding, exists, err := plan.interfaceForTask(workload.Tasks[0].ID); err != nil || !exists || len(binding.TaskIDs) != 1 || binding.TaskIDs[0] != workload.Tasks[0].ID {
		t.Fatalf("durable consumer interface=%+v exists=%t error=%v", binding, exists, err)
	}
}

func TestDurableStateInterfaceFlowsToDirectReaderButNotTransitiveReader(t *testing.T) {
	t.Parallel()
	workload := directedStateWorkload(t,
		"Retain the accepted value between requests.",
		"Read the retained value.",
		"Summarize the direct reader result.",
	)
	capabilities := directCodingCapabilityGraph{
		workload.Tasks[0].RequirementID: nil,
		workload.Tasks[1].RequirementID: {{
			RequirementID: workload.Tasks[0].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(1),
			Purpose:       workload.Tasks[0].RequirementQuote,
		}},
		workload.Tasks[2].RequirementID: {{
			RequirementID: workload.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
			Purpose:       workload.Tasks[1].RequirementQuote,
		}},
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan = bindTestServiceStateInterfaces(
		t, workload, capabilities, plan, testStringServiceStateField("retained"),
	)

	writerInterface, writerExists, err := plan.interfaceForTask(workload.Tasks[0].ID)
	if err != nil || !writerExists {
		t.Fatalf("durable writer interface exists=%t error=%v", writerExists, err)
	}
	readerInterface, readerExists, err := plan.interfaceForTask(workload.Tasks[1].ID)
	if err != nil || !readerExists || readerInterface.ID != writerInterface.ID {
		t.Fatalf("direct reader interface=%+v exists=%t error=%v", readerInterface, readerExists, err)
	}
	if _, exists, err := plan.interfaceForTask(workload.Tasks[2].ID); err != nil || exists {
		t.Fatalf("transitive reader received durable interface: exists=%t error=%v", exists, err)
	}

	bindings := []phpServiceFeatureBinding{
		{TaskID: workload.Tasks[0].ID},
		{TaskID: workload.Tasks[1].ID, StateClassName: "FeatureState102"},
		{TaskID: workload.Tasks[2].ID, StateClassName: "FeatureState103"},
	}
	shared, err := phpServiceDependencyUsesSharedState(bindings[1], bindings[0], plan)
	if err != nil || !shared {
		t.Fatalf("PHP/Laravel direct writer-reader state projection=%t error=%v", shared, err)
	}
	shared, err = phpServiceDependencyUsesSharedState(bindings[2], bindings[1], plan)
	if err != nil || shared {
		t.Fatalf("PHP/Laravel transitive state projection=%t error=%v", shared, err)
	}
}

func TestDurableStateInterfaceKeepsDisconnectedWriterReaderPairsSeparate(t *testing.T) {
	t.Parallel()
	workload, capabilities, plan := unrelatedServiceStateComponentsFixture(t)
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 ||
		!sameStrings(components[0].TaskIDs, []string{workload.Tasks[0].ID, workload.Tasks[1].ID}) ||
		!sameStrings(components[1].TaskIDs, []string{workload.Tasks[2].ID, workload.Tasks[3].ID}) {
		t.Fatalf("disconnected direct state interfaces=%+v", components)
	}
}

func TestDurableStateInterfaceRejectsOverlappingDirectChannels(t *testing.T) {
	t.Parallel()
	workload := directedStateWorkload(t,
		"Retain the first accepted value between requests.",
		"Retain a second value that reads the first value between requests.",
	)
	capabilities := directCodingCapabilityGraph{
		workload.Tasks[0].RequirementID: nil,
		workload.Tasks[1].RequirementID: {{
			RequirementID: workload.Tasks[0].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(1),
			Purpose:       workload.Tasks[0].RequirementQuote,
		}},
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan.ByTask[workload.Tasks[1].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired

	_, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err == nil || !strings.Contains(err.Error(), "overlapping direct durable interfaces") {
		t.Fatalf("overlapping state interfaces error=%v", err)
	}
}

func directedStateWorkload(t *testing.T, needs ...string) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	requirements := make([]assemblyline.Requirement, len(needs))
	for index, need := range needs {
		requirements[index] = assemblyline.Requirement{
			ID:          fmt.Sprintf("requirement_%03d", index+1),
			SourceQuote: need,
		}
	}
	workload, err := assemblyline.FreezeApplicationWorkload(assemblyline.ApplicationSpecification{
		Surface: assemblyline.ApplicationSurfaceService, ProductQuote: "state projection fixture",
		Requirements: requirements,
	})
	if err != nil {
		t.Fatal(err)
	}
	return workload
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
