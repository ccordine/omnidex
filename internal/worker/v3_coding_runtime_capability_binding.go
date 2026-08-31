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
		return program, nil
	}
	return stack.BindRuntimeCapabilities(program, graph)
}

func directCodingRequirementsFromFrozenWorkload(
	workload assemblyline.FrozenApplicationWorkload,
) []assemblyline.Requirement {
	requirements := make([]assemblyline.Requirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		requirements[index] = assemblyline.Requirement{
			ID: task.RequirementID, SourceQuote: task.RequirementQuote,
		}
	}
	return requirements
}
