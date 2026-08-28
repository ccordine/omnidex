package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingServiceStateInterfaceBinding struct {
	ID      string
	TaskIDs []string
	Input   assemblyline.ApplicationServiceStateInterfaceInput
	Result  assemblyline.ApplicationServiceStateInterfaceResult
}

type directCodingServiceStateComponent struct {
	ID      string
	TaskIDs []string
	Input   assemblyline.ApplicationServiceStateInterfaceInput
}

func (s *directCodingSession) resolveServiceStateInterfaces(
	runtime typedWorkerRuntime,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	if len(components) == 0 {
		plan.Interfaces = nil
		plan.InterfaceByTask = nil
		return plan, nil
	}
	model, err := s.workerModel(station.CodingServiceStateInterface)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return resolveDirectCodingServiceStateInterfaces(
		runtime, model, workload, capabilities, plan, identities,
	)
}

func resolveDirectCodingServiceStateInterfaces(
	runtime typedWorkerRuntime,
	model string,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceStatePlan, error) {
	components, err := directCodingServiceStateComponents(workload, capabilities, plan)
	if err != nil {
		return directCodingServiceStatePlan{}, err
	}
	if len(components) == 0 {
		plan.Interfaces = nil
		plan.InterfaceByTask = nil
		return plan, nil
	}
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	plan.Interfaces = make([]directCodingServiceStateInterfaceBinding, 0, len(components))
	plan.InterfaceByTask = make(map[string]string)
	for _, component := range components {
		job, err := assemblyline.NewApplicationServiceStateInterfaceJob(component.Input)
		if err != nil {
			return directCodingServiceStatePlan{}, err
		}
		result, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceStateInterfaceResult](
			runtime, model, "application_service_"+component.ID,
			job, identities,
			func(value assemblyline.ApplicationServiceStateInterfaceResult) error {
				return value.ValidateFor(component.Input)
			},
		)
		if err != nil {
			return directCodingServiceStatePlan{}, fmt.Errorf(
				"resolve service state interface %s: %w", component.ID, err,
			)
		}
		plan.Interfaces = append(plan.Interfaces, directCodingServiceStateInterfaceBinding{
			ID: component.ID, TaskIDs: append([]string(nil), component.TaskIDs...),
			Input: component.Input, Result: result,
		})
		for _, taskID := range component.TaskIDs {
			plan.InterfaceByTask[taskID] = component.ID
		}
	}
	if err := plan.ValidateInterfacesFor(workload, capabilities); err != nil {
		return directCodingServiceStatePlan{}, err
	}
	return plan, nil
}

func directCodingServiceStateComponents(
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	plan directCodingServiceStatePlan,
) ([]directCodingServiceStateComponent, error) {
	if err := plan.ValidateFor(workload); err != nil {
		return nil, err
	}
	workloadInput, err := applicationWorkloadInputFromFrozen(workload)
	if err != nil {
		return nil, err
	}
	requirements := make([]assemblyline.Requirement, 0, len(workload.Tasks))
	indexByRequirement := make(map[string]int, len(workload.Tasks))
	for index, task := range workload.Tasks {
		requirements = append(requirements, assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		})
		indexByRequirement[task.RequirementID] = index
	}
	if err := validateDirectCodingCapabilityGraph(requirements, capabilities); err != nil {
		return nil, err
	}
	adjacency := make([]map[int]struct{}, len(workload.Tasks))
	for index := range adjacency {
		adjacency[index] = make(map[int]struct{})
	}
	for ownerID, dependencies := range capabilities {
		owner := indexByRequirement[ownerID]
		for _, dependency := range dependencies {
			provider := indexByRequirement[dependency.RequirementID]
			adjacency[owner][provider] = struct{}{}
			adjacency[provider][owner] = struct{}{}
		}
	}
	visited := make([]bool, len(workload.Tasks))
	components := make([]directCodingServiceStateComponent, 0)
	for start := range workload.Tasks {
		if visited[start] {
			continue
		}
		queue := []int{start}
		visited[start] = true
		indices := make([]int, 0)
		hasDurable := false
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			indices = append(indices, current)
			if plan.ByTask[workload.Tasks[current].ID] ==
				assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
				hasDurable = true
			}
			for candidate := range workload.Tasks {
				if _, linked := adjacency[current][candidate]; linked && !visited[candidate] {
					visited[candidate] = true
					queue = append(queue, candidate)
				}
			}
		}
		if !hasDurable {
			continue
		}
		component := directCodingServiceStateComponent{
			ID: fmt.Sprintf("state_interface_%03d", len(components)+1),
			Input: assemblyline.ApplicationServiceStateInterfaceInput{
				ProductContext: workload.ProductQuote,
			},
		}
		for _, index := range indices {
			task := workload.Tasks[index]
			authority, err := assemblyline.ProjectApplicationTaskRuntimeAuthority(
				workloadInput, workload, task.ID,
			)
			if err != nil {
				return nil, err
			}
			need, err := assemblyline.ProjectApplicationServiceStateInterfaceNeed(authority)
			if err != nil {
				return nil, err
			}
			component.TaskIDs = append(component.TaskIDs, task.ID)
			component.Input.Needs = append(component.Input.Needs, need)
		}
		if err := component.Input.Validate(); err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, nil
}
