package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func directCodingDeploymentLifecycleManifest(
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	workspaceSHA256 string,
	hasState bool,
) (queue.GeneratedWorkloadDeploymentLifecycleManifest, error) {
	if hasState && descriptor.MigrationScript == "" {
		return queue.GeneratedWorkloadDeploymentLifecycleManifest{}, fmt.Errorf(
			"stateful deployment lifecycle requires one registered migration operation",
		)
	}
	slots := []queue.GeneratedWorkloadDeploymentLifecycleSlot{
		queue.GeneratedDeploymentSlotBuild,
		queue.GeneratedDeploymentSlotInitialStart,
	}
	if hasState {
		slots = append(slots, queue.GeneratedDeploymentSlotMigrate)
	}
	slots = append(slots, queue.GeneratedDeploymentSlotInitialObserve)
	if hasState {
		slots = append(slots, queue.GeneratedDeploymentSlotStateWrite)
	}
	slots = append(slots,
		queue.GeneratedDeploymentSlotRestart,
		queue.GeneratedDeploymentSlotRestartStart,
		queue.GeneratedDeploymentSlotFinalObserve,
	)
	if hasState {
		slots = append(slots, queue.GeneratedDeploymentSlotStateRead)
	}
	commands := make([]queue.GeneratedWorkloadDeploymentExecutionCommand, len(slots))
	for index, slot := range slots {
		kind, err := directCodingDeploymentKindForSlot(slot)
		if err != nil {
			return queue.GeneratedWorkloadDeploymentLifecycleManifest{}, err
		}
		command, err := directCodingDeploymentCommand(kind, project, descriptor, environment)
		if err != nil {
			return queue.GeneratedWorkloadDeploymentLifecycleManifest{}, err
		}
		commands[index] = queue.GeneratedWorkloadDeploymentExecutionCommand{
			Slot: slot, WorkspaceSHA256: workspaceSHA256,
			CommandSHA256: directCodingDigest(strings.Join(
				append([]string{command.Program}, command.Args...), " ",
			)),
		}
	}
	return queue.GeneratedWorkloadDeploymentLifecycleManifest{
		Schema:   queue.GeneratedWorkloadDeploymentLifecycleManifestV1,
		Commands: commands,
	}, nil
}

func directCodingDeploymentRollbackPlan(
	project string,
	descriptor directCodingDeploymentDescriptor,
	environment map[string]string,
	workspaceSHA256 string,
	stateMarkerSHA256 string,
) (queue.GeneratedWorkloadDeploymentRollbackPlan, error) {
	command, err := directCodingDeploymentCommand(
		directCodingDeploymentRollback, project, descriptor, environment,
	)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackPlan{}, err
	}
	plan := queue.GeneratedWorkloadDeploymentRollbackPlan{
		Policy:                  queue.GeneratedWorkloadDeploymentRollbackDestroyFirstV1,
		MaxAttempts:             queue.MaxGeneratedWorkloadDeploymentRollbackAttempts,
		ComposeProject:          project,
		ResourceObservation:     queue.GeneratedWorkloadDeploymentRollbackResourcesV1,
		RequireContainerAbsence: true, RequireNetworkAbsence: true,
		RequireVolumeAbsence: true, StateMarkerSHA256: stateMarkerSHA256,
		Execution: queue.GeneratedWorkloadDeploymentExecutionCommand{
			Slot:            queue.GeneratedDeploymentSlotRollback,
			WorkspaceSHA256: workspaceSHA256,
			CommandSHA256: directCodingDigest(strings.Join(
				append([]string{command.Program}, command.Args...), " ",
			)),
		},
	}
	plan.PostconditionJSON, plan.PostconditionSHA256, err =
		queue.CanonicalGeneratedWorkloadDeploymentRollbackPostcondition(plan)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentRollbackPlan{}, err
	}
	return plan, nil
}

func directCodingDeploymentStateMarkerSHA256(
	program directCodingProgram,
	hasState bool,
) (string, error) {
	if !hasState {
		return "", nil
	}
	storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
	if err != nil {
		return "", err
	}
	if !storage.RequiresPostgreSQL() || storage.Namespace == "" {
		return "", fmt.Errorf("stateful deployment rollback requires exact durable-state marker authority")
	}
	return directCodingDigest(strings.Join([]string{
		"omnidex.deployment-state-marker.v1", storage.Namespace + ":verification",
		"__omnidex_storage_verification__",
	}, "\x00")), nil
}

func directCodingDeploymentKindForSlot(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (directCodingDeploymentCommandKind, error) {
	switch slot {
	case queue.GeneratedDeploymentSlotBuild:
		return directCodingDeploymentBuild, nil
	case queue.GeneratedDeploymentSlotInitialStart, queue.GeneratedDeploymentSlotRestartStart:
		return directCodingDeploymentStart, nil
	case queue.GeneratedDeploymentSlotMigrate:
		return directCodingDeploymentMigrate, nil
	case queue.GeneratedDeploymentSlotInitialObserve, queue.GeneratedDeploymentSlotFinalObserve:
		return directCodingDeploymentObserve, nil
	case queue.GeneratedDeploymentSlotStateWrite:
		return directCodingDeploymentWrite, nil
	case queue.GeneratedDeploymentSlotRestart:
		return directCodingDeploymentRestart, nil
	case queue.GeneratedDeploymentSlotStateRead:
		return directCodingDeploymentRead, nil
	case queue.GeneratedDeploymentSlotRollback:
		return directCodingDeploymentRollback, nil
	default:
		return "", fmt.Errorf("deployment lifecycle slot %s/%d is unsupported", slot.Name, slot.Ordinal)
	}
}

func directCodingDeploymentManifestCommand(
	manifest queue.GeneratedWorkloadDeploymentLifecycleManifest,
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (queue.GeneratedWorkloadDeploymentExecutionCommand, error) {
	for _, command := range manifest.Commands {
		if command.Slot == slot {
			return command, nil
		}
	}
	return queue.GeneratedWorkloadDeploymentExecutionCommand{}, fmt.Errorf(
		"deployment lifecycle manifest omits %s", slot.Name,
	)
}
