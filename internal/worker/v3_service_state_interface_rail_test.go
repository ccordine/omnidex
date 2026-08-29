package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSharedStateInterfaceBindsReaderAsLoadOnlyAndRejectsSaveCapability(t *testing.T) {
	t.Parallel()
	_, workload := serviceStateWorkloadFixture(t)
	capabilities := inventoryStateCapabilityGraph(workload)
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan = bindTestServiceStateInterfaces(
		t, workload, capabilities, plan,
		[]assemblyline.ApplicationServiceStateField{{
			Name: "state_001", Purpose: "The stored inventory records.",
			Kind: assemblyline.ApplicationServiceStateRecordList,
			RecordFields: []assemblyline.ApplicationServiceStateRecordField{
				{Name: "member_001", Purpose: "The inventory code.", Kind: assemblyline.ApplicationServiceStateString},
				{Name: "member_002", Purpose: "The inventory quantity.", Kind: assemblyline.ApplicationServiceStateInteger},
			},
		}},
	)
	reader := workload.Tasks[1]
	stateInterface, exists, err := plan.interfaceForTask(reader.ID)
	if err != nil || !exists {
		t.Fatalf("reader interface exists=%t error=%v", exists, err)
	}
	readerBinding := phpServiceFeatureBinding{
		TaskID: reader.ID, StateBlockID: "feature.state.002",
		StateClassName: "FeatureState102",
	}
	readerBlock := phpServiceStateFacadeBlock(
		readerBinding, "workload:"+workload.SHA256, stateInterface, false,
	)
	if strings.Contains(readerBlock.API, "function save") ||
		strings.Contains(readerBlock.Static, "function save") ||
		!strings.Contains(readerBlock.API, "function load") {
		t.Fatalf("request-local reader received mutation authority: %+v", readerBlock)
	}
	writerBlock := phpServiceStateFacadeBlock(
		phpServiceFeatureBinding{
			TaskID: workload.Tasks[0].ID, StateBlockID: "feature.state.001",
			StateClassName: "FeatureState101",
		}, "workload:"+workload.SHA256, stateInterface, true,
	)
	if !strings.Contains(writerBlock.API, "function save") ||
		!strings.Contains(writerBlock.Static, "function save") {
		t.Fatalf("durable writer lost mutation authority: %+v", writerBlock)
	}
	fragmentInput := assemblyline.FragmentGenerationInput{
		Language: "php", Dialect: "PHP 8.3",
		Signature: "function feature102(TaskInput $input, array $dependencies): TaskResult",
		Behavior:  "Return a view of the shared durable values.",
		Capabilities: []string{
			phpServiceTaskResultAPI(), readerBlock.API,
		},
	}
	bad := `function feature102(TaskInput $input, array $dependencies): TaskResult {
    FeatureState102::save(['records' => []]);
    return TaskResult::success();
}`
	if _, err := validateDirectCodingPHPFragment(fragmentInput, bad); err == nil ||
		!strings.Contains(err.Error(), "undeclared static method save") {
		t.Fatalf("load-only reader save error=%v", err)
	}

	projected, err := plan.projectTask(reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	contextValue, err := assemblyline.ProjectApplicationTaskContext(
		workload, reader.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{Workload: workload, ServiceState: projected}
	if err := validateFocusedPHPServiceStateLifetime(program, contextValue); err != nil {
		t.Fatalf("focused reader interface validation reconstructed unrelated state: %v", err)
	}
	if len(projected.ByTask) != 1 || len(projected.InterfaceByTask) != 1 ||
		len(projected.Interfaces) != 1 ||
		projected.InterfaceByTask[reader.ID] != stateInterface.ID {
		t.Fatalf("focused interface projection=%+v", projected)
	}
}

func TestServiceStateRuntimeQualifiesEmptyAndRejectsIncompatibleShapes(t *testing.T) {
	t.Parallel()
	verifier := phpServiceStateVerifierSource("workload:fixture")
	for _, required := range []string{
		"RuntimeState::assertShape([], $shape);",
		"['unknown' => true]",
		"['records' => [['unknown' => 'value']]]",
		"['records' => [['rank' => 'wrong-kind']]]",
		"['enabled' => 'wrong-kind']",
		"if (!$rejected)",
	} {
		if !strings.Contains(verifier, required) {
			t.Fatalf("state verifier omitted %q", required)
		}
	}
	for name, runtime := range map[string]string{
		"generic PHP": phpServiceStateRuntimeSource(),
		"Laravel":     laravelStateRuntimeSource(),
	} {
		if strings.Contains(runtime, "%!") {
			t.Fatalf("%s state runtime contains an invalid formatted placeholder", name)
		}
		for _, required := range []string{
			"public static function assertShape", "array_key_exists($field, $schema)",
			"!isset($fields[$name])", "Durable state value violates its exact interface.",
		} {
			if !strings.Contains(runtime, required) {
				t.Fatalf("%s state runtime omitted %q", name, required)
			}
		}
	}
}

func TestSharedStateInterfaceResolutionRunsAfterCapabilityClosureAndBeforeSource(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_driver_plan.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	capabilities := strings.Index(source, "s.deriveRequirementCapabilities(")
	stateInterface := strings.Index(source, "s.resolveServiceStateInterfaces(")
	compile := strings.Index(source, "compileDirectCodingServiceProgram(")
	if capabilities < 0 || stateInterface <= capabilities || compile <= stateInterface {
		t.Fatalf(
			"shared state interface order capabilities=%d interface=%d compile=%d",
			capabilities, stateInterface, compile,
		)
	}
}

func inventoryStateCapabilityGraph(
	workload assemblyline.FrozenApplicationWorkload,
) directCodingCapabilityGraph {
	return directCodingCapabilityGraph{
		workload.Tasks[0].RequirementID: nil,
		workload.Tasks[1].RequirementID: {{
			RequirementID: workload.Tasks[0].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(1),
			Purpose:       workload.Tasks[0].RequirementQuote,
		}},
	}
}

func unrelatedServiceStateComponentsFixture(t *testing.T) (
	assemblyline.FrozenApplicationWorkload,
	directCodingCapabilityGraph,
	directCodingServiceStatePlan,
) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceService,
		ProductQuote: "operations service",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Record a warehouse item between requests."},
			{ID: "requirement_002", SourceQuote: "Display the current warehouse items."},
			{ID: "requirement_003", SourceQuote: "Reserve a clinic time between requests."},
			{ID: "requirement_004", SourceQuote: "Display the current clinic reservations."},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{RequirementID: "requirement_001", CapabilityID: genericApplicationCapabilityID(1), Purpose: specification.Requirements[0].SourceQuote}},
		"requirement_003": nil,
		"requirement_004": {{RequirementID: "requirement_003", CapabilityID: genericApplicationCapabilityID(3), Purpose: specification.Requirements[2].SourceQuote}},
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan.ByTask[workload.Tasks[2].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	return workload, capabilities, plan
}
