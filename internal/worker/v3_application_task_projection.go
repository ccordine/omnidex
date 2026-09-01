package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func projectDirectCodingApplicationTaskStage(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) (directCodingProgram, error) {
	var zero directCodingProgram
	stack := program.Project.Stack
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return zero, fmt.Errorf("application task context differs from program workload authority")
	}
	staticFiles, err := cloneValidatedDirectCodingStaticFiles(stack, program.StaticFiles)
	if err != nil {
		return zero, fmt.Errorf("project application task static-file authority: %w", err)
	}
	requirementRelations, err := program.RequirementRelations.projectTask(
		program.Workload, context.Task.TaskID,
	)
	if err != nil {
		return zero, err
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

	generated := make(map[string]string, len(generatedBlocks))
	for _, ref := range generatedBlocks {
		if source := program.Generated[ref.Block.ID]; strings.TrimSpace(source) != "" {
			generated[ref.Block.ID] = source
		}
	}
	stage := directCodingProgram{
		Project:              program.Project,
		Workload:             program.Workload,
		RequirementRelations: requirementRelations,
		TargetTree:           program.TargetTree, Coverage: program.Coverage,
		Source:      assemblyline.SourceBlueprint{Documents: documents},
		StaticFiles: staticFiles,
		Generated:   generated,
	}
	if err := stack.ValidateBlueprint(stage.Source); err != nil {
		return zero, fmt.Errorf("validate application task stage: %w", err)
	}
	return stage, nil
}
