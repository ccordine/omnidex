package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func bindDirectCodingRuntimeCapabilities(
	stack directCodingProjectStack,
	program directCodingProgram,
	graph directCodingRuntimeCapabilityGraph,
) (directCodingProgram, error) {
	if stack.RuntimeCapabilities == nil || stack.BindRuntimeCapabilities == nil {
		if stack.RuntimeCapabilities != nil || stack.BindRuntimeCapabilities != nil {
			return directCodingProgram{}, fmt.Errorf(
				"project stack %s must bind runtime capability registry and source projection together",
				stack.ID,
			)
		}
		requirements, err := directCodingRequirementsFromFrozenWorkload(program.Workload)
		if err != nil {
			return directCodingProgram{}, err
		}
		if err := validateEmptyDirectCodingRuntimeCapabilityGraph(
			requirements, graph,
		); err != nil {
			return directCodingProgram{}, fmt.Errorf(
				"project stack %s does not register runtime capabilities: %w",
				stack.ID, err,
			)
		}
		return program, nil
	}
	candidates, err := stack.RuntimeCapabilities()
	if err != nil {
		return directCodingProgram{}, err
	}
	requirements, err := directCodingRequirementsFromFrozenWorkload(program.Workload)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingRuntimeCapabilityGraph(requirements, candidates, graph); err != nil {
		return directCodingProgram{}, err
	}
	bound, err := stack.BindRuntimeCapabilities(program, graph)
	if err != nil {
		return directCodingProgram{}, err
	}
	if bound.StackID != program.StackID || bound.Workload.SHA256 != program.Workload.SHA256 {
		return directCodingProgram{}, fmt.Errorf(
			"project stack %s runtime capability binding changed program identity",
			stack.ID,
		)
	}
	return bound, nil
}

func validateEmptyDirectCodingRuntimeCapabilityGraph(
	requirements []assemblyline.Requirement,
	graph directCodingRuntimeCapabilityGraph,
) error {
	if len(graph) != len(requirements) {
		return fmt.Errorf(
			"runtime capability graph=%d does not cover requirements=%d",
			len(graph), len(requirements),
		)
	}
	for _, requirement := range requirements {
		selected, exists := graph[requirement.ID]
		if !exists {
			return fmt.Errorf("runtime capability graph omits requirement %s", requirement.ID)
		}
		if len(selected) != 0 {
			return fmt.Errorf(
				"requirement %s names runtime capabilities without a registered stack boundary",
				requirement.ID,
			)
		}
	}
	return nil
}

func directCodingRequirementsFromFrozenWorkload(
	workload assemblyline.FrozenApplicationWorkload,
) ([]assemblyline.Requirement, error) {
	if err := assemblyline.ValidateFrozenApplicationWorkload(workload); err != nil {
		return nil, err
	}
	requirements := make([]assemblyline.Requirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		requirements[index] = assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		}
	}
	return requirements, nil
}
