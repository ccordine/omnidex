package worker

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func projectDirectCodingApplicationTaskStage(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) (directCodingProgram, error) {
	var zero directCodingProgram
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return zero, err
	}
	if err := stack.ValidateBlueprint(program.Source); err != nil {
		return zero, err
	}
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return zero, fmt.Errorf("application task context differs from program workload authority")
	}
	included, err := directCodingTaskStageBlockIDs(program.Source, context.Task.TaskID)
	if err != nil {
		return zero, err
	}
	generatedBlocks, err := directCodingTaskGeneratedBlockRefs(program.Source, context.Task.TaskID)
	if err != nil {
		return zero, err
	}
	documents := make([]assemblyline.SourceDocument, 0, 3)
	for _, document := range program.Source.Documents {
		blocks := make([]assemblyline.SourceBlock, 0, len(document.Blocks))
		for _, block := range document.Blocks {
			if _, exists := included[block.ID]; exists {
				blocks = append(blocks, block)
			}
		}
		if len(blocks) == 0 {
			continue
		}
		document.Blocks = blocks
		scopedPreambles := make([]assemblyline.SourcePreamble, 0, 1)
		for _, preamble := range document.ScopedPreambles {
			if preamble.TaskID == context.Task.TaskID {
				scopedPreambles = append(scopedPreambles, preamble)
			}
		}
		document.ScopedPreambles = scopedPreambles
		documents = append(documents, document)
	}

	requiredStatic := make(map[string]struct{}, len(stack.TaskStageStaticPaths))
	for _, path := range stack.TaskStageStaticPaths {
		requiredStatic[path] = struct{}{}
	}
	optionalStatic := make(map[string]struct{}, len(stack.TaskStageOptionalStaticPaths))
	for _, path := range stack.TaskStageOptionalStaticPaths {
		optionalStatic[path] = struct{}{}
	}
	staticFiles := make([]directCodingFileTask, 0, len(requiredStatic)+len(optionalStatic))
	for _, file := range program.StaticFiles {
		_, required := requiredStatic[file.Path]
		_, optional := optionalStatic[file.Path]
		if required || optional {
			staticFiles = append(staticFiles, file)
			if required {
				delete(requiredStatic, file.Path)
			}
		}
	}
	if len(requiredStatic) != 0 {
		return zero, fmt.Errorf("application task %s stage lacks required toolchain files", context.Task.TaskID)
	}

	generated := make(map[string]string, len(generatedBlocks))
	for _, ref := range generatedBlocks {
		if source := program.Generated[ref.Block.ID]; strings.TrimSpace(source) != "" {
			generated[ref.Block.ID] = source
		}
	}
	serviceEndpoints := directCodingServiceEndpointPlan{}
	if len(program.ServiceEndpoints.Requirements) != 0 {
		serviceEndpoints, err = program.ServiceEndpoints.projectTask(context.Task.TaskID)
		if err != nil {
			return zero, err
		}
	}
	serviceState := directCodingServiceStatePlan{}
	if len(program.ServiceState.ByTask) != 0 {
		serviceState, err = program.ServiceState.projectTask(context.Task.TaskID)
		if err != nil {
			return zero, err
		}
	}
	stage := directCodingProgram{
		StackID: program.StackID, VersionProfileID: program.VersionProfileID,
		Workload:   program.Workload,
		TargetTree: program.TargetTree, Coverage: program.Coverage,
		ServiceState:     serviceState,
		ServiceEndpoints: serviceEndpoints,
		Source:           assemblyline.SourceBlueprint{Documents: documents},
		StaticFiles:      staticFiles, Generated: generated,
	}
	if stack.ProjectTaskStaticFiles != nil {
		projected, projectionErr := stack.ProjectTaskStaticFiles(program, stage)
		if projectionErr != nil {
			return zero, fmt.Errorf("project application task static files: %w", projectionErr)
		}
		if err := validateTaskStageStaticFileProjection(stage.StaticFiles, projected); err != nil {
			return zero, err
		}
		stage.StaticFiles = projected
	}
	if err := stack.ValidateBlueprint(stage.Source); err != nil {
		return zero, fmt.Errorf("validate application task stage: %w", err)
	}
	if !reflect.DeepEqual(stage.Workload, program.Workload) {
		return zero, fmt.Errorf("application task stage changed frozen workload authority")
	}
	return stage, nil
}

func validateTaskStageStaticFileProjection(
	before []directCodingFileTask,
	after []directCodingFileTask,
) error {
	expected := make(map[string]struct{}, len(before))
	for _, file := range before {
		if _, duplicate := expected[file.Path]; duplicate {
			return fmt.Errorf("application task static files repeat %s before projection", file.Path)
		}
		expected[file.Path] = struct{}{}
	}
	for _, file := range after {
		if _, exists := expected[file.Path]; !exists {
			return fmt.Errorf("application task static projection introduced path %s", file.Path)
		}
		if strings.TrimSpace(file.Content) == "" {
			return fmt.Errorf("application task static projection emptied path %s", file.Path)
		}
		delete(expected, file.Path)
	}
	if len(expected) != 0 {
		return fmt.Errorf("application task static projection removed %d code-owned paths", len(expected))
	}
	return nil
}
