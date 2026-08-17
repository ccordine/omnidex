package worker

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationTaskLifecycleHooks struct {
	BeginTask  func(assemblyline.ApplicationTaskContext) error
	BuildBlock func(
		assemblyline.ApplicationTaskContext,
		*directCodingProgram,
		assemblyline.TypeScriptBlock,
	) (string, error)
	CompleteTask func(assemblyline.ApplicationTaskContext, map[string]string) error
	FinalStage   func(*directCodingProgram) error
}

func runDirectCodingApplicationTaskLifecycle(
	input assemblyline.ApplicationWorkloadDraftInput,
	frozen assemblyline.FrozenApplicationWorkload,
	program *directCodingProgram,
	hooks directCodingApplicationTaskLifecycleHooks,
) error {
	if program == nil {
		return fmt.Errorf("application task lifecycle requires one program")
	}
	if hooks.BuildBlock == nil || hooks.FinalStage == nil {
		return fmt.Errorf("application task lifecycle requires generation, verification, and final-stage hooks")
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		return err
	}
	if !reflect.DeepEqual(program.Workload, frozen) {
		return fmt.Errorf("application task lifecycle program workload differs from frozen authority")
	}
	if err := program.TypeScript.Validate(); err != nil {
		return err
	}
	if len(program.Generated) != 0 {
		return fmt.Errorf("application task lifecycle requires an empty generated-source set")
	}

	err := executeDirectCodingApplicationWorkload(
		input, frozen,
		func(context assemblyline.ApplicationTaskContext) error {
			if hooks.BeginTask != nil {
				if err := hooks.BeginTask(context); err != nil {
					return err
				}
			}
			stage, projectErr := projectDirectCodingApplicationTaskStage(*program, context)
			if projectErr != nil {
				return projectErr
			}
			featureID, acceptanceID, identityErr := applicationTaskBlockIDs(context.Task.TaskID)
			if identityErr != nil {
				return identityErr
			}
			for _, blockID := range []string{featureID, acceptanceID} {
				block, exists := directCodingTypeScriptBlueprintBlock(stage.TypeScript, blockID)
				if !exists || !block.Generated() {
					return fmt.Errorf("application task %s lacks generated block %s", context.Task.TaskID, blockID)
				}
				source, generateErr := hooks.BuildBlock(context, &stage, block)
				if generateErr != nil {
					return fmt.Errorf("generate application task block %s: %w", blockID, generateErr)
				}
				if strings.TrimSpace(source) == "" {
					return fmt.Errorf("application task block %s generated empty source", blockID)
				}
				stage.Generated[blockID] = source
			}
			if validationErr := validateApplicationTaskGeneratedSet(stage.Generated, featureID, acceptanceID); validationErr != nil {
				return validationErr
			}
			program.Generated[featureID] = stage.Generated[featureID]
			program.Generated[acceptanceID] = stage.Generated[acceptanceID]
			if hooks.CompleteTask != nil {
				if err := hooks.CompleteTask(context, map[string]string{
					featureID:    stage.Generated[featureID],
					acceptanceID: stage.Generated[acceptanceID],
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	if err := hooks.FinalStage(program); err != nil {
		return fmt.Errorf("verify complete application workload: %w", err)
	}
	return nil
}

func applicationTaskBlockIDs(taskID string) (string, string, error) {
	sequenceText := strings.TrimPrefix(taskID, "task_")
	sequence, err := strconv.Atoi(sequenceText)
	if err != nil || sequence < 1 || taskID != fmt.Sprintf("task_%03d", sequence) {
		return "", "", fmt.Errorf("application task identity %q is not canonical", taskID)
	}
	suffix := fmt.Sprintf("%03d", sequence)
	return "feature." + suffix, "acceptance." + suffix, nil
}

func validateApplicationTaskGeneratedSet(generated map[string]string, allowed ...string) error {
	if len(generated) != len(allowed) {
		return fmt.Errorf("application task verification changed source outside the current task")
	}
	for _, blockID := range allowed {
		if strings.TrimSpace(generated[blockID]) == "" {
			return fmt.Errorf("application task verification omitted current block %s", blockID)
		}
	}
	return nil
}
