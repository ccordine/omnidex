package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestSharedStateInterfaceBindsReaderAsLoadOnlyAndRejectsSaveCapability(t *testing.T) {
	t.Parallel()
	input, workload := serviceStateWorkloadFixture(t)
	capabilities := inventoryStateCapabilityGraph(workload)
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] =
		assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan = bindTestServiceStateInterfaces(
		t, workload, capabilities, plan,
		[]assemblyline.ApplicationServiceStateField{{
			Name: "records", Kind: assemblyline.ApplicationServiceStateRecordList,
			RecordFields: []assemblyline.ApplicationServiceStateRecordField{
				{Name: "code", Kind: assemblyline.ApplicationServiceStateString},
				{Name: "quantity", Kind: assemblyline.ApplicationServiceStateInteger},
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
		input, workload, reader.ID,
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

func TestSharedStateInterfaceResolvesOneRawLeafWithinEachDurableComponent(t *testing.T) {
	t.Parallel()
	workload, capabilities, plan := unrelatedServiceStateComponentsFixture(t)
	calls := 0
	prompts := make([]string, 0, 2)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 7, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
			calls++
			prompts = append(prompts, prompt)
			if model != "state-interface-model" {
				return "", fmt.Errorf("unexpected model %q", model)
			}
			switch {
			case strings.Contains(prompt, "does the directly related behavior authority require any durable root state field"):
				if strings.Contains(prompt, `"accepted_fields":[]`) {
					return "", fmt.Errorf("state field coverage received an empty accepted set")
				}
				return assemblyline.ApplicationNoUncoveredStateField, nil
			case strings.Contains(prompt, "canonical lowercase snake-case name for the earliest necessary durable root state field"):
				if strings.Contains(prompt, "clinic") {
					return "reservations", nil
				}
				return "items", nil
			case strings.Contains(prompt, "what registered data kind must the focused durable root state field use"):
				return string(assemblyline.ApplicationServiceStateRecordList), nil
			case strings.Contains(prompt, "does the focused record-list field require any scalar record member"):
				if strings.Contains(prompt, `"accepted_record_fields":[]`) {
					return "", fmt.Errorf("record field coverage received an empty accepted set")
				}
				return assemblyline.ApplicationNoUncoveredRecordField, nil
			case strings.Contains(prompt, "canonical lowercase snake-case name for the earliest necessary scalar member"):
				return "identifier", nil
			case strings.Contains(prompt, "what registered scalar data kind must the focused record member use"):
				return string(assemblyline.ApplicationServiceStateString), nil
			default:
				return "", fmt.Errorf("unexpected state-interface leaf prompt: %s", prompt)
			}
		}),
	}
	resolved, err := resolveDirectCodingServiceStateInterfaces(
		runtime, "state-interface-model", workload, capabilities, plan, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 12 || len(resolved.Interfaces) != 2 {
		t.Fatalf("semantic calls=%d interfaces=%+v", calls, resolved.Interfaces)
	}
	for _, prompt := range prompts {
		if strings.Contains(prompt, "clinic") && strings.Contains(prompt, "warehouse") {
			t.Fatalf("state component prompt crossed unrelated behavior boundaries: %q", prompt)
		}
	}
	for _, task := range workload.Tasks {
		for _, prompt := range prompts {
			if strings.Contains(prompt, task.ID) || strings.Contains(prompt, task.RequirementID) {
				t.Fatalf("state interface prompt exposed code-owned identity %s", task.ID)
			}
		}
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
	input := assemblyline.ApplicationWorkloadDraftInput{
		Surface:      assemblyline.ApplicationSurfaceService,
		ProductQuote: "operations service",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "Record a warehouse item between requests."},
			{ID: "requirement_002", SourceQuote: "Display the current warehouse items."},
			{ID: "requirement_003", SourceQuote: "Reserve a clinic time between requests."},
			{ID: "requirement_004", SourceQuote: "Display the current clinic reservations."},
		},
	}
	draft := assemblyline.ApplicationWorkloadDraft{
		Schema: assemblyline.ApplicationWorkloadDraftSchemaV1,
		Tasks: []assemblyline.ApplicationWorkloadTaskDraft{
			{RequirementID: "requirement_001", Objective: "Persist one warehouse item.", RequiredBehaviors: []string{"Retain the item."}, AcceptanceCriteria: []string{"A later lookup sees the item."}},
			{RequirementID: "requirement_002", Objective: "Present warehouse items.", RequiredBehaviors: []string{"Read the retained items."}, AcceptanceCriteria: []string{"Current items are visible."}},
			{RequirementID: "requirement_003", Objective: "Persist one clinic reservation.", RequiredBehaviors: []string{"Retain the reservation."}, AcceptanceCriteria: []string{"A later lookup sees the reservation."}},
			{RequirementID: "requirement_004", Objective: "Present clinic reservations.", RequiredBehaviors: []string{"Read the retained reservations."}, AcceptanceCriteria: []string{"Current reservations are visible."}},
		},
	}
	workload, err := assemblyline.FreezeApplicationWorkload(input, draft)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		"requirement_001": nil,
		"requirement_002": {{RequirementID: "requirement_001", CapabilityID: genericApplicationCapabilityID(1), Purpose: input.Requirements[0].SourceQuote}},
		"requirement_003": nil,
		"requirement_004": {{RequirementID: "requirement_003", CapabilityID: genericApplicationCapabilityID(3), Purpose: input.Requirements[2].SourceQuote}},
	}
	plan := testRequestLocalServiceStatePlan(workload)
	plan.ByTask[workload.Tasks[0].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	plan.ByTask[workload.Tasks[2].ID] = assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired
	return workload, capabilities, plan
}
