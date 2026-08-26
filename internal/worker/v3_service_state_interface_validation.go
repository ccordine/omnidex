package worker

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (plan directCodingServiceStatePlan) ValidateInterfacesFor(
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
) error {
	expected, err := directCodingServiceStateComponents(workload, capabilities,
		directCodingServiceStatePlan{
			WorkloadSHA256: plan.WorkloadSHA256,
			ByTask:         plan.ByTask,
		})
	if err != nil {
		return err
	}
	if len(plan.Interfaces) != len(expected) {
		return fmt.Errorf(
			"service state interface plan has %d interfaces for %d code-derived components",
			len(plan.Interfaces), len(expected),
		)
	}
	expectedTasks := make(map[string]string)
	for index, component := range expected {
		binding := plan.Interfaces[index]
		if binding.ID != component.ID || !reflect.DeepEqual(binding.TaskIDs, component.TaskIDs) ||
			!reflect.DeepEqual(binding.Input, component.Input) {
			return fmt.Errorf(
				"service state interface %d differs from its code-derived component authority",
				index+1,
			)
		}
		if err := binding.Result.ValidateFor(binding.Input); err != nil {
			return fmt.Errorf("service state interface %s: %w", binding.ID, err)
		}
		for _, taskID := range binding.TaskIDs {
			expectedTasks[taskID] = binding.ID
		}
	}
	if len(plan.InterfaceByTask) != len(expectedTasks) {
		return fmt.Errorf("service state interface task projection differs from code-owned components")
	}
	for taskID, interfaceID := range expectedTasks {
		if plan.InterfaceByTask[taskID] != interfaceID {
			return fmt.Errorf("service state interface task projection differs from code-owned components")
		}
	}
	return nil
}

func (plan directCodingServiceStatePlan) interfaceForTask(
	taskID string,
) (directCodingServiceStateInterfaceBinding, bool, error) {
	interfaceID, exists := plan.InterfaceByTask[taskID]
	if !exists {
		return directCodingServiceStateInterfaceBinding{}, false, nil
	}
	for _, binding := range plan.Interfaces {
		if binding.ID == interfaceID {
			return binding, true, nil
		}
	}
	return directCodingServiceStateInterfaceBinding{}, false, fmt.Errorf(
		"service state task %s references missing interface %s", taskID, interfaceID,
	)
}
