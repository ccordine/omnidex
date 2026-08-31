package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingAssemblyFromProgram(program directCodingProgram) (directCodingAssembly, error) {
	files := append([]directCodingFileTask(nil), program.StaticFiles...)
	composed, err := composeDirectCodingSourceProgram(program)
	if err != nil {
		return directCodingAssembly{}, err
	}
	for _, source := range composed {
		files = append(files, directCodingFileTask{
			Path: source.Path, Content: []byte(source.Source), Mode: 0o644,
		})
	}
	byPath := make(map[string]directCodingFileTask, len(files))
	for _, file := range files {
		if _, exists := byPath[file.Path]; exists {
			return directCodingAssembly{}, fmt.Errorf("compiled program repeats source path %q", file.Path)
		}
		byPath[file.Path] = file
	}
	deletePaths := append([]string(nil), program.DeletePaths...)
	for _, targetPath := range program.TargetTree.Paths {
		if _, exists := byPath[targetPath]; !exists {
			return directCodingAssembly{}, fmt.Errorf(
				"target-tree path %q has no compiled source", targetPath,
			)
		}
	}
	// The path-only tree declares workload artifacts only. Runtime shells,
	// manifests, generated application composition, and adapter tests are
	// deterministic adapter output, not omissions from the model tree.
	assembly := directCodingAssembly{
		VersionProfileID: program.VersionProfileID,
		Files:            files,
		DeletePaths:      deletePaths,
	}
	if err := assembly.normalize(); err != nil {
		return directCodingAssembly{}, err
	}
	return assembly, nil
}

func directCodingSourceBlueprintBlockCount(blueprint assemblyline.SourceBlueprint) int {
	count := 0
	for _, document := range blueprint.Documents {
		count += len(document.Blocks)
	}
	return count
}
