package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationTaskLifecycleHooks struct {
	BuildBlock func(
		assemblyline.ApplicationTaskContext,
		*directCodingProgram,
		assemblyline.SourceBlockRef,
	) (string, error)
}

func runDirectCodingApplicationTaskLifecycle(
	frozen assemblyline.FrozenApplicationWorkload,
	program *directCodingProgram,
	hooks directCodingApplicationTaskLifecycleHooks,
) error {
	if program == nil {
		return fmt.Errorf("application task lifecycle requires one program")
	}
	if hooks.BuildBlock == nil {
		return fmt.Errorf("application task lifecycle requires one source-generation hook")
	}
	if len(program.Generated) != 0 {
		return fmt.Errorf("application task lifecycle requires an empty generated-source set")
	}

	err := executeIndependentDirectCodingApplicationWorkload(
		frozen,
		func(context assemblyline.ApplicationTaskContext) error {
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
			for _, blockID := range expectedIDs {
				program.Generated[blockID] = stage.Generated[blockID]
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func validateApplicationTaskGeneratedSet(generated map[string]string, allowed ...string) error {
	if len(generated) != len(allowed) {
		return fmt.Errorf("application task generation changed source outside the current task")
	}
	for _, blockID := range allowed {
		if strings.TrimSpace(generated[blockID]) == "" {
			return fmt.Errorf("application task generation omitted current block %s", blockID)
		}
	}
	return nil
}
