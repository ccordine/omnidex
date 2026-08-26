package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func validatePHPServiceStateLifetime(
	workload assemblyline.FrozenApplicationWorkload,
	plan directCodingServiceStatePlan,
) error {
	_, err := deriveDirectCodingServiceStoragePlan(workload, plan)
	return err
}

func validateFocusedPHPServiceStateLifetime(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) error {
	if _, err := focusedPHPServiceStorage(program, context); err != nil {
		return err
	}
	return validateFocusedPHPServiceStateInterface(program.ServiceState, context.Task.TaskID)
}

func validateFocusedPHPServiceStateInterface(
	plan directCodingServiceStatePlan,
	taskID string,
) error {
	if len(plan.InterfaceByTask) == 0 && len(plan.Interfaces) == 0 {
		return nil
	}
	if len(plan.InterfaceByTask) != 1 || len(plan.Interfaces) != 1 {
		return fmt.Errorf("focused PHP HTTP state requires exactly one interface projection")
	}
	interfaceID, exists := plan.InterfaceByTask[taskID]
	if !exists {
		return fmt.Errorf("focused PHP HTTP state interface omits task %s", taskID)
	}
	binding := plan.Interfaces[0]
	if binding.ID != interfaceID {
		return fmt.Errorf("focused PHP HTTP state interface identity differs from task projection")
	}
	member := false
	for _, boundTaskID := range binding.TaskIDs {
		if boundTaskID == taskID {
			member = true
			break
		}
	}
	if !member {
		return fmt.Errorf("focused PHP HTTP state interface lost task membership")
	}
	if err := binding.Result.ValidateFor(binding.Input); err != nil {
		return fmt.Errorf("focused PHP HTTP state interface %s: %w", binding.ID, err)
	}
	return nil
}

func focusedPHPServiceStorage(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) (directCodingServiceStorageKind, error) {
	plan := program.ServiceState
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != program.Workload.SHA256 ||
		context.WorkloadSHA256 != program.Workload.SHA256 {
		return "", fmt.Errorf("focused PHP HTTP state plan differs from workload authority")
	}
	if len(plan.ByTask) != 1 {
		return "", fmt.Errorf("focused PHP HTTP state plan requires exactly one task decision")
	}
	lifetime, exists := plan.ByTask[context.Task.TaskID]
	if !exists {
		return "", fmt.Errorf("focused PHP HTTP state plan omits task %s", context.Task.TaskID)
	}
	switch lifetime {
	case assemblyline.ApplicationServiceStateRequestLocalOnly:
		return directCodingServiceStorageRequestLocal, nil
	case assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired:
		return directCodingServiceStoragePostgreSQL, nil
	default:
		return "", fmt.Errorf(
			"focused PHP HTTP task %s has unsupported service state lifetime %q",
			context.Task.TaskID, lifetime,
		)
	}
}

func phpServiceProgramRequiresPostgreSQL(program directCodingProgram) (bool, error) {
	required := map[string]bool{
		phpServiceStateMigrationPath:    false,
		phpServiceStateMigrationRunner:  false,
		phpServiceStateVerificationPath: false,
		phpServiceStateVerificationEnv:  false,
		phpServiceStateDeploymentEnv:    false,
	}
	present := 0
	for _, file := range program.StaticFiles {
		if _, tracked := required[file.Path]; !tracked {
			continue
		}
		if required[file.Path] {
			return false, fmt.Errorf("PHP HTTP state artifacts repeat %s", file.Path)
		}
		required[file.Path] = true
		present++
	}
	if present == 0 {
		for _, task := range program.Workload.Tasks {
			lifetime, exists := program.ServiceState.ByTask[task.ID]
			if !exists {
				continue
			}
			if lifetime == assemblyline.ApplicationServiceStateCrossRequestAuthorityRequired {
				return false, fmt.Errorf(
					"PHP HTTP durable task %s lacks its code-owned PostgreSQL artifacts", task.ID,
				)
			}
		}
		return false, nil
	}
	if present != len(required) {
		return false, fmt.Errorf(
			"PHP HTTP durable state requires %d code-owned artifacts, found %d",
			len(required), present,
		)
	}
	if len(program.ServiceState.ByTask) == len(program.Workload.Tasks) {
		storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
		if err != nil {
			return false, err
		}
		if !storage.RequiresPostgreSQL() {
			return false, fmt.Errorf("PHP HTTP request-local project contains unused PostgreSQL artifacts")
		}
	}
	return true, nil
}
