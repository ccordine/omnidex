package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func testServiceEndpointPlan(
	t *testing.T,
	stackID string,
	workload assemblyline.FrozenApplicationWorkload,
	requirements map[string]assemblyline.ApplicationServiceEndpointRequirement,
	contracts map[string]assemblyline.ApplicationServiceEndpointContract,
) directCodingServiceEndpointPlan {
	t.Helper()
	stack, err := directCodingProjectStackByID(stackID)
	if err != nil {
		t.Fatal(err)
	}
	plan := directCodingServiceEndpointPlan{
		WorkloadSHA256: workload.SHA256,
		ProductContext: workload.ProductQuote,
		Requirements:   make(map[string]assemblyline.ApplicationServiceEndpointRequirement, len(requirements)),
		ByTask:         make(map[string]assemblyline.ApplicationServiceEndpointContract),
	}
	for _, task := range workload.Tasks {
		requirement, exists := requirements[task.ID]
		if !exists {
			t.Fatalf("endpoint fixture omits task %s", task.ID)
		}
		plan.Requirements[task.ID] = requirement
		if requirement == assemblyline.ApplicationServiceSupportOnly {
			if _, exists := contracts[task.ID]; exists {
				t.Fatalf("support-only endpoint fixture %s has a semantic contract", task.ID)
			}
			continue
		}
		contract, exists := contracts[task.ID]
		if !exists {
			t.Fatalf("endpoint-required fixture %s omits its semantic contract", task.ID)
		}
		plan.ByTask[task.ID] = contract
	}
	if len(plan.Requirements) != len(workload.Tasks) {
		t.Fatalf("endpoint fixture has %d decisions for %d tasks", len(plan.Requirements), len(workload.Tasks))
	}
	if err := validateServiceEndpointPlanForHTTPStack(stack, workload, plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func testHTTPServiceEndpointContract(
	method assemblyline.ApplicationServiceEndpointMethod,
	route string,
	requestMedia assemblyline.ApplicationServiceEndpointMedia,
	responseMedia assemblyline.ApplicationServiceEndpointMedia,
	status int,
) assemblyline.ApplicationServiceEndpointContract {
	return assemblyline.ApplicationServiceEndpointContract{
		Schema:   assemblyline.ApplicationServiceEndpointContractSchemaV1,
		Exposure: assemblyline.ApplicationServiceEndpointPublic, Method: method,
		RouteTemplate: route, RequestMedia: requestMedia, ResponseMedia: responseMedia,
		SuccessStatus: status,
	}
}
