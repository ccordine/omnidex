package worker

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// This projection is used only by deterministic verification and publication.
// Generation continues to receive its isolated, single-task projection.
func projectDirectCodingAcceptedTaskStage(program directCodingProgram) (directCodingProgram, error) {
	if _, err := directCodingProgramHasCompleteGeneratedSet(program); err != nil {
		return directCodingProgram{}, err
	}
	if err := program.RequirementRelations.validateCompleteFor(program.Workload); err != nil {
		return directCodingProgram{}, err
	}
	tasks := make(map[string]struct{})
	blocks := make(map[string]struct{})
	for _, task := range program.Workload.Tasks {
		refs, err := directCodingTaskGeneratedBlockRefs(program.Source, task.ID)
		if err != nil {
			return directCodingProgram{}, err
		}
		count := 0
		for _, ref := range refs {
			if _, exists := program.Generated[ref.Block.ID]; exists {
				count++
			}
		}
		if count == 0 {
			continue
		}
		if count != len(refs) {
			return directCodingProgram{}, fmt.Errorf("accepted task %s has an incomplete generated-source set", task.ID)
		}
		included, err := directCodingTaskStageBlockIDs(program.Source, task.ID)
		if err != nil {
			return directCodingProgram{}, err
		}
		tasks[task.ID] = struct{}{}
		for id := range included {
			blocks[id] = struct{}{}
		}
	}
	if len(tasks) == 0 {
		return directCodingProgram{}, fmt.Errorf("publication requires at least one verified task")
	}
	stage := program
	stage.Source = assemblyline.SourceBlueprint{}
	stage.Generated = make(map[string]string, len(program.Generated))
	for id, source := range program.Generated {
		stage.Generated[id] = source
	}
	for _, original := range program.Source.Documents {
		document := original
		document.Blocks = nil
		document.ScopedPreambles = nil
		for _, block := range original.Blocks {
			if _, include := blocks[block.ID]; include {
				document.Blocks = append(document.Blocks, block)
			}
		}
		if len(document.Blocks) == 0 {
			continue
		}
		for _, preamble := range original.ScopedPreambles {
			if _, include := tasks[preamble.TaskID]; include {
				document.ScopedPreambles = append(document.ScopedPreambles, preamble)
			}
		}
		stage.Source.Documents = append(stage.Source.Documents, document)
	}
	stage.TargetTree.Paths = nil
	for _, file := range program.Coverage.Files {
		for _, taskID := range file.TaskIDs {
			if _, include := tasks[taskID]; include {
				stage.TargetTree.Paths = append(stage.TargetTree.Paths, file.Path)
				break
			}
		}
	}
	stage.ProtectedPaths, stage.RequiredPaths, stage.DeletePaths = nil, nil, nil
	var err error
	stage.StaticFiles, err = cloneValidatedDirectCodingStaticFiles(program.Project.Stack, program.StaticFiles)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := stage.Project.Stack.ValidateBlueprint(stage.Source); err != nil {
		return directCodingProgram{}, fmt.Errorf("validate accepted task projection: %w", err)
	}
	return stage, nil
}

// Only complete original documents can leave the experiment. A partial shared
// document (including an isolated compiler harness) has no write authority.
func directCodingTaskPublicationAssembly(program, stage directCodingProgram) (directCodingAssembly, error) {
	if err := program.Source.Validate(); err != nil {
		return directCodingAssembly{}, err
	}
	assembly, err := directCodingAssemblyFromProgram(stage)
	if err != nil {
		return directCodingAssembly{}, err
	}
	if err := validateDirectCodingProjectedProgramAssembly(stage, assembly); err != nil {
		return directCodingAssembly{}, err
	}
	originals := make(map[string]assemblyline.SourceDocument)
	owners := make(map[string]string)
	for _, document := range program.Source.Documents {
		originals[document.ID] = document
		for _, block := range document.Blocks {
			owners[block.ID] = document.ID
		}
	}
	ready := make(map[string]assemblyline.SourceDocument)
	for _, document := range stage.Source.Documents {
		original, exists := originals[document.ID]
		if !exists {
			return directCodingAssembly{}, fmt.Errorf("publication contains unknown source document %s", document.ID)
		}
		if directCodingSourceDocumentsEqual(original, document) {
			ready[document.ID] = original
		}
	}
	for changed := true; changed; {
		changed = false
		for id, document := range ready {
			for _, block := range document.Blocks {
				for _, dependency := range block.DependsOn {
					if _, exists := ready[owners[dependency]]; !exists {
						delete(ready, id)
						changed = true
					}
				}
			}
		}
	}
	paths := make(map[string]struct{})
	generated := false
	for _, document := range ready {
		paths[document.Path] = struct{}{}
		for _, block := range document.Blocks {
			generated = generated || block.Generated()
		}
	}
	if !generated {
		return directCodingAssembly{}, nil
	}
	for _, file := range program.StaticFiles {
		paths[file.Path] = struct{}{}
	}
	publication := directCodingAssembly{}
	for _, file := range assembly.Files {
		if _, include := paths[file.Path]; include {
			publication.Files = append(publication.Files, file)
		}
	}
	return publication, nil
}

func directCodingSourceDocumentsEqual(left, right assemblyline.SourceDocument) bool {
	if !slices.Equal(left.ScopedPreambles, right.ScopedPreambles) {
		return false
	}
	left.ScopedPreambles, right.ScopedPreambles = nil, nil
	return reflect.DeepEqual(left, right)
}
