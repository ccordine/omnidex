package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPHPHTTPLifecycleDerivesTwoUnrelatedStateAuthorities(t *testing.T) {
	t.Parallel()
	workload, capabilities, state := unrelatedServiceStateComponentsFixture(t)
	state = bindTestServiceStateInterfaces(t, workload, capabilities, state,
		[]assemblyline.ApplicationServiceStateField{
			{Name: "state_001", Purpose: "The stored reference.", Kind: assemblyline.ApplicationServiceStateString},
			{Name: "state_002", Purpose: "The stored label.", Kind: assemblyline.ApplicationServiceStateString},
		},
	)
	bindings := []phpServiceFeatureBinding{
		phpServiceLifecycleTestBinding(workload, 4, assemblyline.ApplicationServiceEndpointGET,
			"/clinic/current", assemblyline.ApplicationServiceEndpointMediaNone,
			assemblyline.ApplicationServiceEndpointHTML),
		phpServiceLifecycleTestBinding(workload, 1, assemblyline.ApplicationServiceEndpointPOST,
			"/warehouse/entries", assemblyline.ApplicationServiceEndpointJSON,
			assemblyline.ApplicationServiceEndpointJSON),
		phpServiceLifecycleTestBinding(workload, 3, assemblyline.ApplicationServiceEndpointPUT,
			"/clinic/reservations", assemblyline.ApplicationServiceEndpointForm,
			assemblyline.ApplicationServiceEndpointJSON),
		phpServiceLifecycleTestBinding(workload, 2, assemblyline.ApplicationServiceEndpointGET,
			"/warehouse/current", assemblyline.ApplicationServiceEndpointMediaNone,
			assemblyline.ApplicationServiceEndpointJSON),
	}
	plan, err := derivePHPServiceHTTPLifecyclePlan(workload, capabilities, state, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Lifecycles) != 2 || len(plan.Blockers) != 0 {
		t.Fatalf("HTTP lifecycle plan=%+v", plan)
	}
	source, err := phpServiceHTTPLifecycleSource(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"performHttpRequest('POST', '/warehouse/entries'",
		"performHttpRequest('GET', '/warehouse/current'",
		"performHttpRequest('PUT', '/clinic/reservations'",
		"performHttpRequest('GET', '/clinic/current'",
		"verifyLifecycleSentinel(", "'application/x-www-form-urlencoded'",
		"odx-1-", "odx-2-",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("HTTP lifecycle source omitted %q:\n%s", required, source)
		}
	}
	if strings.Count(source, "verificationLifecycleRequest(") != 4 ||
		strings.Count(source, "odx-1-") != 8 || strings.Count(source, "odx-2-") != 8 ||
		strings.Contains(source, "random_") {
		t.Fatalf("HTTP lifecycle sentinels are not two deterministic phases:\n%s", source)
	}
}

func TestPHPHTTPLifecycleRejectsAResponseMissingOneOfMultipleSentinels(t *testing.T) {
	t.Parallel()
	runtime := phpServiceHTTPLifecycleRuntime()
	start := strings.Index(runtime, "function verifyLifecycleSentinel(")
	end := strings.Index(runtime, "function verificationContainsExactValue(")
	if start < 0 || end <= start {
		t.Fatal("HTTP lifecycle runtime omits its bounded sentinel verifier")
	}
	verifier := runtime[start:end]
	for _, required := range []string{
		"foreach ($sentinels as $sentinel)",
		"$missing[] = $sentinel;",
		"if ($missing !== [])",
		"dropped one or more sentinels",
	} {
		if !strings.Contains(verifier, required) {
			t.Fatalf("complete sentinel verifier omitted %q:\n%s", required, verifier)
		}
	}
	if strings.Contains(verifier, "return;") || strings.Contains(verifier, "did not expose any sentinel") {
		t.Fatalf("sentinel verifier can still pass after observing only one leaf:\n%s", verifier)
	}
}

func TestPHPHTTPLifecycleFailsLoudlyWithoutExactSemanticAuthority(t *testing.T) {
	t.Parallel()
	workload, capabilities, state := unrelatedServiceStateComponentsFixture(t)
	state = bindTestServiceStateInterfaces(t, workload, capabilities, state,
		[]assemblyline.ApplicationServiceStateField{{
			Name: "state_001", Purpose: "Whether the operation is enabled.",
			Kind: assemblyline.ApplicationServiceStateBoolean,
		}},
	)
	bindings := []phpServiceFeatureBinding{
		phpServiceLifecycleTestBinding(workload, 1, assemblyline.ApplicationServiceEndpointPOST,
			"/first", assemblyline.ApplicationServiceEndpointJSON,
			assemblyline.ApplicationServiceEndpointJSON),
		phpServiceLifecycleTestBinding(workload, 2, assemblyline.ApplicationServiceEndpointGET,
			"/second", assemblyline.ApplicationServiceEndpointMediaNone,
			assemblyline.ApplicationServiceEndpointJSON),
		phpServiceLifecycleTestBinding(workload, 3, assemblyline.ApplicationServiceEndpointPUT,
			"/third", assemblyline.ApplicationServiceEndpointJSON,
			assemblyline.ApplicationServiceEndpointJSON),
		phpServiceLifecycleTestBinding(workload, 4, assemblyline.ApplicationServiceEndpointGET,
			"/fourth/{identity}", assemblyline.ApplicationServiceEndpointMediaNone,
			assemblyline.ApplicationServiceEndpointJSON),
	}
	plan, err := derivePHPServiceHTTPLifecyclePlan(workload, capabilities, state, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Lifecycles) != 0 || len(plan.Blockers) != 2 {
		t.Fatalf("unverifiable HTTP lifecycle plan=%+v", plan)
	}
	source, err := phpServiceHTTPLifecycleSource(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"throw new RuntimeException(", "no mechanically verifiable cross-endpoint lifecycle",
		"type-preserving state payload", "parameter-free GET endpoint",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("HTTP lifecycle blocker omitted %q: %s", required, source)
		}
	}
}

func TestPHPSharedStateDependencyLoadsSnapshotWithoutReinvokingWriter(t *testing.T) {
	t.Parallel()
	workload, capabilities, state := unrelatedServiceStateComponentsFixture(t)
	state = bindTestServiceStateInterfaces(t, workload, capabilities, state,
		[]assemblyline.ApplicationServiceStateField{{
			Name: "state_001", Purpose: "The stored entries.",
			Kind: assemblyline.ApplicationServiceStateStringList,
		}},
	)
	writer := phpServiceLifecycleTestBinding(
		workload, 1, assemblyline.ApplicationServiceEndpointPOST, "/entries",
		assemblyline.ApplicationServiceEndpointJSON, assemblyline.ApplicationServiceEndpointJSON,
	)
	reader := phpServiceLifecycleTestBinding(
		workload, 2, assemblyline.ApplicationServiceEndpointGET, "/current",
		assemblyline.ApplicationServiceEndpointMediaNone, assemblyline.ApplicationServiceEndpointJSON,
	)
	byRequirement := map[string]phpServiceFeatureBinding{
		writer.RequirementID: writer, reader.RequirementID: reader,
	}
	requirements := []assemblyline.Requirement{
		{ID: writer.RequirementID, SourceQuote: workload.Tasks[0].RequirementQuote},
		{ID: reader.RequirementID, SourceQuote: workload.Tasks[1].RequirementQuote},
	}
	componentCapabilities := directCodingCapabilityGraph{
		writer.RequirementID: nil,
		reader.RequirementID: capabilities[reader.RequirementID],
	}
	order, err := phpServiceEndpointExecutionOrderWithState(
		reader, requirements, componentCapabilities, byRequirement, state,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 1 || order[0].TaskID != reader.TaskID {
		t.Fatalf("shared-state GET re-invokes producer: %+v", order)
	}
	source, err := phpServiceFeatureInvocationWithState(
		reader, componentCapabilities, byRequirement, state,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "TaskResult::success('', FeatureState102::load())") ||
		strings.Contains(source, "feature101(") {
		t.Fatalf("shared-state capability was not loaded from its durable snapshot:\n%s", source)
	}
}

func phpServiceLifecycleTestBinding(
	workload assemblyline.FrozenApplicationWorkload,
	sequence int,
	method assemblyline.ApplicationServiceEndpointMethod,
	route string,
	requestMedia, responseMedia assemblyline.ApplicationServiceEndpointMedia,
) phpServiceFeatureBinding {
	task := workload.Tasks[sequence-1]
	status := 200
	if method == assemblyline.ApplicationServiceEndpointPOST {
		status = 201
	}
	featureNumber := []string{"101", "102", "201", "202"}[sequence-1]
	return phpServiceFeatureBinding{
		Sequence: sequence, RequirementID: task.RequirementID, TaskID: task.ID,
		FeatureNumber: featureNumber, FeatureName: "feature" + featureNumber,
		StateClassName: "FeatureState" + featureNumber, HasEndpoint: true,
		Endpoint: assemblyline.ApplicationServiceEndpointContract{
			Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
			Exposure: assemblyline.ApplicationServiceEndpointPublic,
			Method:   method, RouteTemplate: route, RequestMedia: requestMedia,
			ResponseMedia: responseMedia, SuccessStatus: status,
		},
		Fixture: phpServiceInputFixture{Method: string(method), Path: route},
	}
}
