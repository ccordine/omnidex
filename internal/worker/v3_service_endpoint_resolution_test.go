package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestServiceEndpointResolutionClassifiesOnlyConsumedTaskAndContractsUnconsumedTask(t *testing.T) {
	t.Parallel()
	_, frozen, _ := applicationTaskLifecycleFixture(t)
	requirementCalls := 0
	leafCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, model, prompt string) (string, error) {
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
				if !strings.Contains(prompt, "DIRECT_CAPABILITY_CONSUMER:\ntrue") {
					t.Fatalf("endpoint requirement prompt omitted code-owned consumer fact: %s", prompt)
				}
				if !strings.Contains(prompt, frozen.Tasks[1].RequirementQuote) ||
					strings.Contains(prompt, frozen.Tasks[0].RequirementQuote) {
					t.Fatalf("endpoint requirement prompt was not isolated to the consumed task: %s", prompt)
				}
				requirementCalls++
				return string(assemblyline.ApplicationServiceSupportOnly), nil
			case "exposure-model":
				leafCalls++
				return string(assemblyline.ApplicationServiceEndpointPublic), nil
			case "method-model":
				leafCalls++
				return string(assemblyline.ApplicationServiceEndpointGET), nil
			case "route-model":
				leafCalls++
				return "/records", nil
			case "response-model":
				leafCalls++
				return string(assemblyline.ApplicationServiceEndpointHTML), nil
			default:
				return "", fmt.Errorf("unexpected service model %q", model)
			}
		}),
	}
	capabilities := directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: {{
			RequirementID: frozen.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
			Purpose:       frozen.Tasks[1].RequirementQuote,
		}},
		frozen.Tasks[1].RequirementID: nil,
	}
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveDirectCodingServiceEndpointPlan(
		runtime, "requirement-model", directCodingServiceEndpointLeafModels{
			Exposure: "exposure-model", Method: "method-model", Route: "route-model",
			RequestMedia: "request-model", ResponseMedia: "response-model", SuccessStatus: "status-model",
		}, stack, frozen, capabilities, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if requirementCalls != 1 || leafCalls != 4 ||
		len(plan.Requirements) != len(frozen.Tasks) || len(plan.ByTask) != 1 {
		t.Fatalf(
			"requirement calls=%d plan=%+v", requirementCalls, plan,
		)
	}
	if plan.Requirements[frozen.Tasks[0].ID] != assemblyline.ApplicationServiceEndpointRequired ||
		plan.Requirements[frozen.Tasks[1].ID] != assemblyline.ApplicationServiceSupportOnly {
		t.Fatalf("endpoint requirement decisions=%+v", plan.Requirements)
	}
	if got := plan.ByTask[frozen.Tasks[0].ID]; got.Schema != assemblyline.ApplicationServiceEndpointContractSchemaV1 ||
		got.RouteTemplate != "/records" || got.Method != assemblyline.ApplicationServiceEndpointGET ||
		got.RequestMedia != assemblyline.ApplicationServiceEndpointMediaNone ||
		got.ResponseMedia != assemblyline.ApplicationServiceEndpointHTML || got.SuccessStatus != 200 {
		t.Fatalf("code-composed endpoint=%+v", got)
	}
}

func TestServiceEndpointResolutionMakesNoRequirementCallWithoutCapabilityConsumer(t *testing.T) {
	t.Parallel()
	input, _, _ := applicationTaskLifecycleFixture(t)
	oneRequirement := input
	oneRequirement.Requirements = append(
		[]assemblyline.Requirement(nil), input.Requirements[:1]...,
	)
	frozen, err := assemblyline.FreezeApplicationWorkload(oneRequirement)
	if err != nil {
		t.Fatal(err)
	}
	requirementCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, CorrectionModel: "forbidden-correction",
		Execute: testPortableExecutor(func(_ string, model, _ string) (string, error) {
			switch model {
			case "requirement-model":
				requirementCalls++
				return "", fmt.Errorf("endpoint requirement model must not run without a capability consumer")
			case "exposure-model":
				return string(assemblyline.ApplicationServiceEndpointPublic), nil
			case "method-model":
				return string(assemblyline.ApplicationServiceEndpointGET), nil
			case "route-model":
				return "/records", nil
			case "response-model":
				return string(assemblyline.ApplicationServiceEndpointHTML), nil
			default:
				return "", fmt.Errorf("unexpected service model %q", model)
			}
		}),
	}
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := resolveDirectCodingServiceEndpointPlan(
		runtime, "requirement-model", directCodingServiceEndpointLeafModels{
			Exposure: "exposure-model", Method: "method-model", Route: "route-model",
			RequestMedia: "request-model", ResponseMedia: "response-model", SuccessStatus: "status-model",
		}, stack, frozen, directCodingCapabilityGraph{
			frozen.Tasks[0].RequirementID: nil,
		}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if requirementCalls != 0 {
		t.Fatalf("endpoint requirement model calls=%d, want 0", requirementCalls)
	}
	if got := plan.Requirements[frozen.Tasks[0].ID]; got != assemblyline.ApplicationServiceEndpointRequired {
		t.Fatalf("deterministic endpoint requirement=%q", got)
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
	if err := plan.ValidateFor(frozen); err != nil {
		t.Fatal(err)
	}
	capabilities := directCodingCapabilityGraph{
		frozen.Tasks[0].RequirementID: {{
			RequirementID: frozen.Tasks[1].RequirementID,
			CapabilityID:  genericApplicationCapabilityID(2),
		}},
		frozen.Tasks[1].RequirementID: nil,
	}
	if err := plan.ValidateForCapabilities(frozen, capabilities); err != nil {
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
	if err := overlap.ValidateFor(frozen); err == nil || !strings.Contains(err.Error(), "overlapping") {
		t.Fatalf("overlapping route validation error=%v", err)
	}
	overlap.ByTask = map[string]assemblyline.ApplicationServiceEndpointContract{
		frozen.Tasks[0].ID: testServiceEndpointContract("/records/{record_id}"),
		frozen.Tasks[1].ID: testServiceEndpointContract("/{collection}/active"),
	}
	if err := overlap.ValidateFor(frozen); err == nil || !strings.Contains(err.Error(), "overlapping") {
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
	if err := plan.ValidateFor(frozen); err != nil {
		t.Fatal(err)
	}
	if err := plan.ValidateForCapabilities(frozen, directCodingCapabilityGraph{
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
	if err := plan.ValidateFor(frozen); err == nil || !strings.Contains(err.Error(), "support-only") {
		t.Fatalf("support-only route validation error=%v", err)
	}
	delete(plan.ByTask, frozen.Tasks[1].ID)
	if err := plan.ValidateForCapabilities(frozen, directCodingCapabilityGraph{
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
	if err := plan.ValidateFor(frozen); err == nil {
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
