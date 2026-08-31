package worker

import (
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingAcceptedPartialProgram(
	program directCodingProgram,
) (directCodingProgram, bool) {
	generatedByTask := make(map[string][]string)
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if block.TaskID == "" || !block.Generated() {
				continue
			}
			generatedByTask[block.TaskID] = append(generatedByTask[block.TaskID], block.ID)
		}
	}
	successfulTasks := make(map[string]struct{})
	for taskID, blockIDs := range generatedByTask {
		complete := len(blockIDs) > 0
		for _, blockID := range blockIDs {
			if strings.TrimSpace(program.Generated[blockID]) == "" {
				complete = false
				break
			}
		}
		if complete {
			successfulTasks[taskID] = struct{}{}
		}
	}
	if len(successfulTasks) == 0 {
		return directCodingProgram{}, false
	}

	active := make(map[string]struct{})
	blocks := make(map[string]assemblyline.SourceBlock)
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			blocks[block.ID] = block
			if block.TaskID != "" {
				if _, accepted := successfulTasks[block.TaskID]; !accepted {
					continue
				}
			}
			if block.Generated() && strings.TrimSpace(program.Generated[block.ID]) == "" {
				continue
			}
			active[block.ID] = struct{}{}
		}
	}
	for changed := true; changed; {
		changed = false
		for blockID := range active {
			for _, dependencyID := range blocks[blockID].DependsOn {
				if _, available := active[dependencyID]; available {
					continue
				}
				delete(active, blockID)
				changed = true
				break
			}
		}
	}

	documents := make([]assemblyline.SourceDocument, 0, len(program.Source.Documents))
	documentPaths := make(map[string]struct{})
	for _, document := range program.Source.Documents {
		retainedBlocks := make([]assemblyline.SourceBlock, 0, len(document.Blocks))
		retainedTasks := make(map[string]struct{})
		for _, block := range document.Blocks {
			if _, retained := active[block.ID]; !retained {
				continue
			}
			retainedBlocks = append(retainedBlocks, block)
			if block.TaskID != "" {
				retainedTasks[block.TaskID] = struct{}{}
			}
		}
		if len(retainedBlocks) == 0 {
			continue
		}
		retainedPreambles := make([]assemblyline.SourcePreamble, 0, len(document.ScopedPreambles))
		for _, preamble := range document.ScopedPreambles {
			if _, retained := retainedTasks[preamble.TaskID]; retained {
				retainedPreambles = append(retainedPreambles, preamble)
			}
		}
		document.Blocks = retainedBlocks
		document.ScopedPreambles = retainedPreambles
		documents = append(documents, document)
		documentPaths[document.Path] = struct{}{}
	}

	generated := make(map[string]string)
	for blockID := range active {
		if source := program.Generated[blockID]; strings.TrimSpace(source) != "" {
			generated[blockID] = source
		}
	}
	targetPaths := make([]string, 0, len(program.TargetTree.Paths))
	for _, artifactPath := range program.TargetTree.Paths {
		if _, retained := documentPaths[artifactPath]; retained {
			targetPaths = append(targetPaths, artifactPath)
		}
	}
	program.Source = assemblyline.SourceBlueprint{Documents: documents}
	program.Generated = generated
	program.TargetTree = assemblyline.TargetTree{Paths: targetPaths}
	return program, true
}
