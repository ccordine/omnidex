package worker

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationTaskLifecycleHooks struct {
	BeginTask  func(assemblyline.ApplicationTaskContext) error
	BuildBlock func(
		assemblyline.ApplicationTaskContext,
		*directCodingProgram,
		assemblyline.SourceBlockRef,
	) (string, error)
	VerifyTask   func(assemblyline.ApplicationTaskContext, *directCodingProgram) error
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
	if hooks.BuildBlock == nil || hooks.VerifyTask == nil || hooks.FinalStage == nil {
		return fmt.Errorf("application task lifecycle requires generation, verification, and final-stage hooks")
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		return err
	}
	if !reflect.DeepEqual(program.Workload, frozen) {
		return fmt.Errorf("application task lifecycle program workload differs from frozen authority")
	}
	if err := program.Source.Validate(); err != nil {
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
			refs, refsErr := directCodingTaskGeneratedBlockRefs(stage.Source, context.Task.TaskID)
			if refsErr != nil {
				return refsErr
			}
			expectedIDs := make([]string, 0, len(refs))
			for _, ref := range refs {
				expectedIDs = append(expectedIDs, ref.Block.ID)
				source, generateErr := hooks.BuildBlock(context, &stage, ref)
				if generateErr != nil {
					return fmt.Errorf("generate application task block %s: %w", ref.Block.ID, generateErr)
				}
				if strings.TrimSpace(source) == "" {
					return fmt.Errorf("application task block %s generated empty source", ref.Block.ID)
				}
				stage.Generated[ref.Block.ID] = source
			}
			if validationErr := validateApplicationTaskGeneratedSet(stage.Generated, expectedIDs...); validationErr != nil {
				return validationErr
			}
			if verifyErr := hooks.VerifyTask(context, &stage); verifyErr != nil {
				return fmt.Errorf("verify application task %s: %w", context.Task.TaskID, verifyErr)
			}
			if validationErr := validateApplicationTaskGeneratedSet(stage.Generated, expectedIDs...); validationErr != nil {
				return validationErr
			}
			accepted := make(map[string]string, len(expectedIDs))
			for _, blockID := range expectedIDs {
				program.Generated[blockID] = stage.Generated[blockID]
				accepted[blockID] = stage.Generated[blockID]
			}
			if hooks.CompleteTask != nil {
				if err := hooks.CompleteTask(context, accepted); err != nil {
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
