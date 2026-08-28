package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceEndpointResolutionClassifiesEveryTaskAndContractsOnlyRequiredEndpoints(t *testing.T) {
	t.Parallel()
	input, frozen, _ := applicationTaskLifecycleFixture(t)
	requirementCalls := 0
	leafCalls := map[string]int{}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
			for _, task := range frozen.Tasks {
				if strings.Contains(prompt, task.ID) {
					t.Fatalf("service prompt exposed task identity %s: %s", task.ID, prompt)
				}
			}
			for _, forbidden := range []string{".php", "Dockerfile", "workspace", "command", "tool"} {
				if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
					t.Fatalf("service prompt exposed %q authority: %s", forbidden, prompt)
				}
			}
			switch model {
			case "requirement-model":
				requirement := assemblyline.ApplicationServiceEndpointRequired
				if requirementCalls == 1 {
					requirement = assemblyline.ApplicationServiceSupportOnly
				}
				requirementCalls++
				return fmt.Sprintf(
					`{"schema":"%s","endpoint_requirement":%q}`,
					assemblyline.ApplicationServiceEndpointRequirementSchemaV1, requirement,
				), nil
			case "exposure-model":
				leafCalls[model]++
				return `{"schema":"omnidex.application-service-endpoint-exposure.v1","exposure":"public"}`, nil
			case "method-model":
				leafCalls[model]++
				return `{"schema":"omnidex.application-service-endpoint-method.v1","method":"GET"}`, nil
			case "route-model":
				leafCalls[model]++
				return `{"schema":"omnidex.application-service-endpoint-route-template.v1","route_template":"/records"}`, nil
			case "request-media-model":
				return "", fmt.Errorf("GET request media must be derived without inference")
			case "response-media-model":
				leafCalls[model]++
				return `{"schema":"omnidex.application-service-endpoint-response-media.v1","response_media":"application/json"}`, nil
			case "success-status-model":
				return "", fmt.Errorf("GET success status must be derived without inference")
			default:
				return "", fmt.Errorf("unexpected service model %q", model)
			}
		}),
	}
	capabilities := directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: {{
			RequirementID: frozen.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
			Purpose:       "Use the support result.",
		}},
		frozen.Tasks[1].RequirementID: nil,
	}
	plan, err := resolveDirectCodingServiceEndpointPlan(
		runtime, "requirement-model", directCodingServiceEndpointLeafModels{
			Exposure: "exposure-model", Method: "method-model", Route: "route-model",
			RequestMedia: "request-media-model", ResponseMedia: "response-media-model",
			SuccessStatus: "success-status-model",
		}, input, frozen, capabilities, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if requirementCalls != len(frozen.Tasks) || len(leafCalls) != 4 ||
		len(plan.Requirements) != len(frozen.Tasks) || len(plan.ByTask) != 1 {
		t.Fatalf(
			"requirement calls=%d leaf calls=%v plan=%+v",
			requirementCalls, leafCalls, plan,
		)
	}
	for model, count := range leafCalls {
		if count != 1 {
			t.Fatalf("leaf model %s calls=%d", model, count)
		}
	}
	if got := plan.ByTask[frozen.Tasks[0].ID]; got.Schema != assemblyline.ApplicationServiceEndpointContractSchemaV1 ||
		got.RouteTemplate != "/records" || got.SuccessStatus != 200 {
		t.Fatalf("code-composed endpoint=%+v", got)
	}
}

func TestServiceEndpointResolutionDerivesNoContentStatusWithoutInference(t *testing.T) {
	t.Parallel()
	input, frozen, _ := applicationTaskLifecycleFixture(t)
	runtimeAuthority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
		input, frozen, frozen.Tasks[0].ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := assemblyline.ProjectApplicationServiceEndpointTaskAuthority(
		runtimeAuthority,
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := make(map[string]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, model, _ string, _ map[string]any) (string, error) {
			calls[model]++
			switch model {
			case "exposure-model":
				return `{"schema":"omnidex.application-service-endpoint-exposure.v1","exposure":"public"}`, nil
			case "method-model":
				return `{"schema":"omnidex.application-service-endpoint-method.v1","method":"POST"}`, nil
			case "route-model":
				return `{"schema":"omnidex.application-service-endpoint-route-template.v1","route_template":"/records"}`, nil
			case "request-media-model":
				return `{"schema":"omnidex.application-service-endpoint-request-media.v1","request_media":"application/json"}`, nil
			case "response-media-model":
				return `{"schema":"omnidex.application-service-endpoint-response-media.v1","response_media":"none"}`, nil
			case "success-status-model":
				return "", fmt.Errorf("no-content status must be derived without inference")
			default:
				return "", fmt.Errorf("unexpected model %q", model)
			}
		}),
	}
	contract, err := resolveDirectCodingServiceEndpointContractLeaves(
		runtime,
		directCodingServiceEndpointLeafModels{
			Exposure: "exposure-model", Method: "method-model", Route: "route-model",
			RequestMedia: "request-media-model", ResponseMedia: "response-media-model",
			SuccessStatus: "success-status-model",
		},
		authority, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if contract.SuccessStatus != 204 || calls["success-status-model"] != 0 || len(calls) != 5 {
		t.Fatalf("contract=%+v calls=%v", contract, calls)
	}
}

func TestServiceEndpointPlanRequiresOneNonOverlappingContractPerFrozenTask(t *testing.T) {
	t.Parallel()
	input, frozen, _ := applicationTaskLifecycleFixture(t)
	plan := directCodingServiceEndpointPlan{
		WorkloadSHA256: frozen.SHA256,
		ProductContext: input.ProductQuote,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			frozen.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
			frozen.Tasks[1].ID: assemblyline.ApplicationServiceEndpointRequired,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			frozen.Tasks[0].ID: testServiceEndpointContract("/records/{record_id}"),
			frozen.Tasks[1].ID: testServiceEndpointContract("/groups/{group_id}"),
		},
	}
	if err := plan.ValidateFor(input, frozen); err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: {{
			RequirementID: frozen.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
		}},
		frozen.Tasks[1].RequirementID: nil,
	}
	if err := plan.ValidateForCapabilities(input, frozen, capabilities); err != nil {
		t.Fatal(err)
	}
	projected, err := plan.projectTask(frozen.Tasks[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.ByTask) != 1 || projected.ByTask[frozen.Tasks[1].ID].RouteTemplate != "/groups/{group_id}" {
		t.Fatalf("projected endpoint plan=%+v", projected)
	}

	overlap := plan
	overlap.ByTask = map[string]assemblyline.ApplicationServiceEndpointContract{
		frozen.Tasks[0].ID: testServiceEndpointContract("/records/{record_id}"),
		frozen.Tasks[1].ID: testServiceEndpointContract("/records/{different_name}"),
	}
	if err := overlap.ValidateFor(input, frozen); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("overlapping route validation error=%v", err)
	}
	overlap.ByTask = map[string]assemblyline.ApplicationServiceEndpointContract{
		frozen.Tasks[0].ID: testServiceEndpointContract("/records/{record_id}"),
		frozen.Tasks[1].ID: testServiceEndpointContract("/{collection}/active"),
	}
	if err := overlap.ValidateFor(input, frozen); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("intersecting literal/parameter route validation error=%v", err)
	}
}

func TestServiceEndpointPlanProjectsSupportOnlyTaskWithoutInventingRoute(t *testing.T) {
	t.Parallel()
	input, frozen, _ := applicationTaskLifecycleFixture(t)
	plan := directCodingServiceEndpointPlan{
		WorkloadSHA256: frozen.SHA256,
		ProductContext: input.ProductQuote,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			frozen.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
			frozen.Tasks[1].ID: assemblyline.ApplicationServiceSupportOnly,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			frozen.Tasks[0].ID: testServiceEndpointContract("/records"),
		},
	}
	if err := plan.ValidateFor(input, frozen); err != nil {
		t.Fatal(err)
	}
	if err := plan.ValidateForCapabilities(input, frozen, directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: {{
			RequirementID: frozen.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
		}},
		frozen.Tasks[1].RequirementID: nil,
	}); err != nil {
		t.Fatal(err)
	}
	projected, err := plan.projectTask(frozen.Tasks[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Requirements) != 1 || len(projected.ByTask) != 0 ||
		projected.Requirements[frozen.Tasks[1].ID] != assemblyline.ApplicationServiceSupportOnly {
		t.Fatalf("support-only projection=%+v", projected)
	}
	plan.ByTask[frozen.Tasks[1].ID] = testServiceEndpointContract("/invented")
	if err := plan.ValidateFor(input, frozen); err == nil || !strings.Contains(err.Error(), "support-only") {
		t.Fatalf("support-only route validation error=%v", err)
	}
	delete(plan.ByTask, frozen.Tasks[1].ID)
	if err := plan.ValidateForCapabilities(input, frozen, directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: nil,
		frozen.Tasks[1].RequirementID: nil,
	}); err == nil || !strings.Contains(err.Error(), "no code-derived capability consumer") {
		t.Fatalf("unconsumed support-only validation error=%v", err)
	}
}

func TestServiceEndpointPlanRejectsMissingAndUnknownTaskAuthority(t *testing.T) {
	t.Parallel()
	input, frozen, _ := applicationTaskLifecycleFixture(t)
	plan := directCodingServiceEndpointPlan{
		WorkloadSHA256: frozen.SHA256,
		ProductContext: input.ProductQuote,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			frozen.Tasks[0].ID: assemblyline.ApplicationServiceEndpointRequired,
			"task_unknown":     assemblyline.ApplicationServiceEndpointRequired,
		},
		ByTask: map[string]assemblyline.ApplicationServiceEndpointContract{
			frozen.Tasks[0].ID: testServiceEndpointContract("/records"),
			"task_unknown":     testServiceEndpointContract("/unknown"),
		},
	}
	if err := plan.ValidateFor(input, frozen); err == nil {
		t.Fatal("service endpoint plan accepted missing and unknown task authority")
	}
}

func testServiceEndpointContract(route string) assemblyline.ApplicationServiceEndpointContract {
	return assemblyline.ApplicationServiceEndpointContract{
		Schema:        assemblyline.ApplicationServiceEndpointContractSchemaV1,
		Exposure:      assemblyline.ApplicationServiceEndpointPublic,
		Method:        assemblyline.ApplicationServiceEndpointGET,
		RouteTemplate: route,
		RequestMedia:  assemblyline.ApplicationServiceEndpointMediaNone,
		ResponseMedia: assemblyline.ApplicationServiceEndpointJSON,
		SuccessStatus: 200,
	}
}
