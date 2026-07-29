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
	if err := assembly.normalize(); err != nil {
		return directCodingAssembly{}, err
	}
	return assembly, nil
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
