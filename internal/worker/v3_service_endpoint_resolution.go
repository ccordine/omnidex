package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingServiceEndpointPlan struct {
	WorkloadSHA256 string
	ProductContext string
	Requirements   map[string]assemblyline.ApplicationServiceEndpointRequirement
	ByTask         map[string]assemblyline.ApplicationServiceEndpointContract
}

type directCodingServiceEndpointLeafModels struct {
	Exposure      string
	Method        string
	Route         string
	RequestMedia  string
	ResponseMedia string
	SuccessStatus string
}

func resolveDirectCodingServiceEndpointPlan(
	runtime typedWorkerRuntime,
	requirementModel string,
	leafModels directCodingServiceEndpointLeafModels,
	workloadInput assemblyline.ApplicationWorkloadDraftInput,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceEndpointPlan, error) {
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	plan := directCodingServiceEndpointPlan{
		WorkloadSHA256: workload.SHA256,
		ProductContext: workloadInput.ProductQuote,
		Requirements:   make(map[string]assemblyline.ApplicationServiceEndpointRequirement, len(workload.Tasks)),
		ByTask:         make(map[string]assemblyline.ApplicationServiceEndpointContract, len(workload.Tasks)),
	}
	for _, task := range workload.Tasks {
		runtimeAuthority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
			workloadInput, workload, task.ID,
		)
		if err != nil {
			return directCodingServiceEndpointPlan{}, err
		}
		requirementInput, err := assemblyline.ProjectApplicationServiceEndpointRequirementInput(
			runtimeAuthority,
		)
		if err != nil {
			return directCodingServiceEndpointPlan{}, err
		}
		requirementJob, err := assemblyline.NewApplicationServiceEndpointRequirementJob(requirementInput)
		if err != nil {
			return directCodingServiceEndpointPlan{}, err
		}
		requirement, err := runDirectCodingSemanticLeafCall(
			runtime, requirementModel, "application_service_endpoint_requirement", requirementJob, identities,
			func(raw string) (assemblyline.ApplicationServiceEndpointRequirementResult, error) {
				return assemblyline.DecodeApplicationServiceEndpointRequirementResult(requirementInput, raw)
			},
			func(value assemblyline.ApplicationServiceEndpointRequirementResult) error {
				return value.ValidateFor(requirementInput)
			},
		)
		if err != nil {
			return directCodingServiceEndpointPlan{}, fmt.Errorf(
				"resolve service endpoint requirement for task %s: %w", task.ID, err,
			)
		}
		plan.Requirements[task.ID] = requirement.EndpointRequirement
		if requirement.EndpointRequirement == assemblyline.ApplicationServiceSupportOnly {
			continue
		}
		authority, err := assemblyline.ProjectApplicationServiceEndpointTaskAuthority(runtimeAuthority)
		if err != nil {
			return directCodingServiceEndpointPlan{}, err
		}
		contract, err := resolveDirectCodingServiceEndpointContractLeaves(
			runtime, leafModels, authority, identities,
		)
		if err != nil {
			return directCodingServiceEndpointPlan{}, fmt.Errorf(
				"resolve service endpoint for task %s: %w", task.ID, err,
			)
		}
		plan.ByTask[task.ID] = contract
	}
	if err := plan.ValidateForCapabilities(workloadInput, workload, capabilities); err != nil {
		return directCodingServiceEndpointPlan{}, err
	}
	return plan, nil
}

func (plan directCodingServiceEndpointPlan) ValidateForCapabilities(
	workloadInput assemblyline.ApplicationWorkloadDraftInput,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
) error {
	if err := plan.ValidateFor(workloadInput, workload); err != nil {
		return err
	}
	requirementByTask := make(map[string]string, len(workload.Tasks))
	for _, task := range workload.Tasks {
		requirementByTask[task.ID] = task.RequirementID
	}
	for _, task := range workload.Tasks {
		if plan.Requirements[task.ID] != assemblyline.ApplicationServiceSupportOnly {
			continue
		}
		consumed := false
		for consumerRequirement, dependencies := range capabilities {
			if consumerRequirement == task.RequirementID {
				continue
			}
			for _, dependency := range dependencies {
				if dependency.RequirementID == task.RequirementID {
					consumed = true
					break
				}
			}
			if consumed {
				break
			}
		}
		if !consumed {
			return fmt.Errorf(
				"support-only service task %s has no code-derived capability consumer",
				task.ID,
			)
		}
	}
	for taskID, requirementID := range requirementByTask {
		if _, exists := capabilities[requirementID]; !exists {
			return fmt.Errorf("service capability graph omits task %s requirement %s", taskID, requirementID)
		}
	}
	return nil
}

func (plan directCodingServiceEndpointPlan) ValidateFor(
	workloadInput assemblyline.ApplicationWorkloadDraftInput,
	workload assemblyline.FrozenApplicationWorkload,
) error {
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != workload.SHA256 {
		return fmt.Errorf("service endpoint plan differs from frozen workload authority")
	}
	if len(plan.Requirements) != len(workload.Tasks) {
		return fmt.Errorf(
			"service endpoint plan has %d requirement decisions for %d frozen tasks",
			len(plan.Requirements), len(workload.Tasks),
		)
	}
	if len(plan.ByTask) == 0 {
		return fmt.Errorf("service endpoint plan has no independently addressable HTTP endpoint")
	}
	type acceptedRoute struct {
		taskID   string
		contract assemblyline.ApplicationServiceEndpointContract
	}
	seenRoutes := make([]acceptedRoute, 0, len(plan.ByTask))
	for _, task := range workload.Tasks {
		requirement, exists := plan.Requirements[task.ID]
		if !exists {
			return fmt.Errorf("service endpoint plan omits requirement decision for frozen task %s", task.ID)
		}
		runtimeAuthority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
			workloadInput, workload, task.ID,
		)
		if err != nil {
			return err
		}
		requirementInput, err := assemblyline.ProjectApplicationServiceEndpointRequirementInput(
			runtimeAuthority,
		)
		if err != nil {
			return err
		}
		if err := (assemblyline.ApplicationServiceEndpointRequirementResult{
			Schema:              assemblyline.ApplicationServiceEndpointRequirementSchemaV1,
			EndpointRequirement: requirement,
		}).ValidateFor(requirementInput); err != nil {
			return fmt.Errorf("validate service endpoint requirement for task %s: %w", task.ID, err)
		}
		contract, hasContract := plan.ByTask[task.ID]
		if requirement == assemblyline.ApplicationServiceSupportOnly {
			if hasContract {
				return fmt.Errorf("support-only service task %s has an HTTP endpoint contract", task.ID)
			}
			continue
		}
		if !hasContract {
			return fmt.Errorf("endpoint-required service task %s has no HTTP endpoint contract", task.ID)
		}
		authority, err := assemblyline.ProjectApplicationServiceEndpointTaskAuthority(runtimeAuthority)
		if err != nil {
			return err
		}
		if err := contract.ValidateFor(authority); err != nil {
			return fmt.Errorf("validate service endpoint task %s: %w", task.ID, err)
		}
		for _, previous := range seenRoutes {
			if previous.contract.Method == contract.Method && serviceRouteTemplatesOverlap(
				previous.contract.RouteTemplate, contract.RouteTemplate,
			) {
				return fmt.Errorf(
					"service endpoint tasks %s and %s have overlapping %s route templates",
					previous.taskID, task.ID, contract.Method,
				)
			}
		}
		seenRoutes = append(seenRoutes, acceptedRoute{taskID: task.ID, contract: contract})
	}
	for taskID := range plan.ByTask {
		found := false
		for _, task := range workload.Tasks {
			if task.ID == taskID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("service endpoint plan contains unknown task %s", taskID)
		}
	}
	for taskID := range plan.Requirements {
		found := false
		for _, task := range workload.Tasks {
			if task.ID == taskID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("service endpoint plan contains unknown requirement task %s", taskID)
		}
	}
	return nil
}

func (plan directCodingServiceEndpointPlan) projectTask(
	taskID string,
) (directCodingServiceEndpointPlan, error) {
	requirement, exists := plan.Requirements[taskID]
	if !exists {
		return directCodingServiceEndpointPlan{}, fmt.Errorf(
			"service endpoint plan omits projected task requirement %s", taskID,
		)
	}
	projected := directCodingServiceEndpointPlan{
		WorkloadSHA256: plan.WorkloadSHA256,
		ProductContext: plan.ProductContext,
		Requirements: map[string]assemblyline.ApplicationServiceEndpointRequirement{
			taskID: requirement,
		},
		ByTask: make(map[string]assemblyline.ApplicationServiceEndpointContract, 1),
	}
	if requirement == assemblyline.ApplicationServiceEndpointRequired {
		contract, hasContract := plan.ByTask[taskID]
		if !hasContract {
			return directCodingServiceEndpointPlan{}, fmt.Errorf(
				"service endpoint plan omits projected endpoint contract %s", taskID,
			)
		}
		projected.ByTask[taskID] = contract
	}
	return projected, nil
}

func serviceRouteTemplatesOverlap(left, right string) bool {
	leftSegments := strings.Split(strings.TrimPrefix(left, "/"), "/")
	rightSegments := strings.Split(strings.TrimPrefix(right, "/"), "/")
	if len(leftSegments) != len(rightSegments) {
		return false
	}
	for index := range leftSegments {
		leftParameter := strings.HasPrefix(leftSegments[index], "{") &&
			strings.HasSuffix(leftSegments[index], "}")
		rightParameter := strings.HasPrefix(rightSegments[index], "{") &&
			strings.HasSuffix(rightSegments[index], "}")
		if !leftParameter && !rightParameter && leftSegments[index] != rightSegments[index] {
			return false
		}
	}
	return true
}
