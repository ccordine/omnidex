package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingAssemblyFromProgram(program directCodingProgram) (directCodingAssembly, error) {
	files := append([]directCodingFileTask(nil), program.StaticFiles...)
	composed, err := composeDirectCodingTypeScriptProgram(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	for _, source := range composed {
		files = append(files, directCodingFileTask{Path: source.Path, Content: source.Source})
	}
	assembly := directCodingAssembly{Files: files}
	for _, transition := range program.StructureTransitions {
		switch transition.Kind {
		case assemblyline.TargetTreeEnsureDirectory:
			continue
		case assemblyline.TargetTreeCreate, assemblyline.TargetTreeReconcile:
			if err := requireDirectCodingTargetTreeLeafContent(transition, assembly.Files); err != nil {
				return directCodingAssembly{}, err
			}
		default:
			return directCodingAssembly{}, fmt.Errorf("unsupported target-tree transition %q for path %q", transition.Kind, transition.Path)
		}
	}
	if err := assembly.normalize(); err != nil {
		return directCodingAssembly{}, err
	}
	return assembly, nil
}

// requireDirectCodingTargetTreeLeafContent binds one accepted target-tree file
// leaf to one code-owned content job. The tree decides which artifact exists;
// code decides and validates every filesystem mutation. A declared leaf is
// never silently discarded because the current adapter lacks work for it.
func requireDirectCodingTargetTreeLeafContent(
	transition assemblyline.TargetTreeTransition,
	files []directCodingFileTask,
) error {
	if transition.Path == "" {
		return fmt.Errorf("target-tree transition %q has no target file", transition.Kind)
	}
	for _, file := range files {
		if file.Path == transition.Path {
			return nil
		}
	}
	return fmt.Errorf(
		"target-tree path %q has no code-owned file-content job",
		transition.Path,
	)
}

func composeDirectCodingTypeScriptProgram(program directCodingProgram) ([]assemblyline.ComposedTypeScriptDocument, error) {
	composed := make([]assemblyline.ComposedTypeScriptDocument, 0, len(program.TypeScript.Documents))
	for _, document := range program.TypeScript.Documents {
		source, err := assemblyline.ComposeTypeScriptDocument(document, program.Generated)
		if err != nil {
			return nil, fmt.Errorf("compose deterministic TypeScript document %s: %w", document.ID, err)
		}
		composed = append(composed, source)
	}
	return composed, nil
}

func directCodingTypeScriptBlueprintBlockCount(blueprint assemblyline.TypeScriptBlueprint) int {
	count := 0
	for _, document := range blueprint.Documents {
		count += len(document.Blocks)
	}
	return count
}

func directCodingProgramGraphMetrics(program directCodingProgram) (int, int, error) {
	waves, err := program.TypeScript.BuildWaves()
	return directCodingTypeScriptBlueprintBlockCount(program.TypeScript), len(waves), err
}
