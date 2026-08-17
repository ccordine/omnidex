package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

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
	byPath := make(map[string]directCodingFileTask, len(files))
	for _, file := range files {
		if _, exists := byPath[file.Path]; exists {
			return directCodingAssembly{}, fmt.Errorf("compiled program repeats source path %q", file.Path)
		}
		byPath[file.Path] = file
	}
	for _, transition := range program.StructureTransitions {
		switch transition.Kind {
		case assemblyline.TargetTreeEnsureDirectory:
			continue
		case assemblyline.TargetTreeCreate, assemblyline.TargetTreeReconcile:
			if _, exists := byPath[transition.Path]; !exists {
				if err := requireDirectCodingTargetTreeLeafSource(transition, files); err != nil {
					return directCodingAssembly{}, err
				}
				return directCodingAssembly{}, fmt.Errorf("target-tree source path %q disappeared from compiled program", transition.Path)
			}
		default:
			return directCodingAssembly{}, fmt.Errorf("unsupported target-tree transition %q for path %q", transition.Kind, transition.Path)
		}
	}
	// The path-only tree declares workload artifacts only. Runtime shells,
	// manifests, generated application composition, and adapter tests are
	// deterministic adapter output, not omissions from the model tree.
	assembly := directCodingAssembly{Files: files}
	if err := assembly.normalize(); err != nil {
		return directCodingAssembly{}, err
	}
	return assembly, nil
}

// directCodingAssemblyFilesystemTransitions turns all code-owned output into
// one ordered filesystem workload. The target-tree leaves remain mandatory
// checks, while adapter baseline files are added by code because their paths
// and bytes are already determined by the selected adapter.
func directCodingAssemblyFilesystemTransitions(
	existingPaths []string,
	existingDirectories []string,
	targetTransitions []assemblyline.TargetTreeTransition,
	assembly directCodingAssembly,
) ([]assemblyline.TargetTreeTransition, error) {
	if err := assembly.validate(); err != nil {
		return nil, err
	}
	presentFiles := make(map[string]struct{}, len(existingPaths))
	for _, value := range existingPaths {
		normalized, err := normalizeDirectCodingPath(value)
		if err != nil {
			return nil, fmt.Errorf("existing file path: %w", err)
		}
		presentFiles[normalized] = struct{}{}
	}
	presentDirectories := make(map[string]struct{}, len(existingDirectories))
	for _, value := range existingDirectories {
		normalized, err := normalizeDirectCodingPath(value)
		if err != nil {
			return nil, fmt.Errorf("existing directory path: %w", err)
		}
		presentDirectories[normalized] = struct{}{}
	}
	files := make(map[string]struct{}, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = struct{}{}
	}
	for _, transition := range targetTransitions {
		switch transition.Kind {
		case assemblyline.TargetTreeEnsureDirectory:
			continue
		case assemblyline.TargetTreeCreate, assemblyline.TargetTreeReconcile:
			if _, exists := files[transition.Path]; !exists {
				return nil, fmt.Errorf("target-tree path %q has no code-owned source job", transition.Path)
			}
		default:
			return nil, fmt.Errorf("unsupported target-tree transition %q for path %q", transition.Kind, transition.Path)
		}
	}
	neededDirectories := make(map[string]struct{})
	for _, file := range assembly.Files {
		for directory := path.Dir(file.Path); directory != "."; directory = path.Dir(directory) {
			if _, exists := presentDirectories[directory]; !exists {
				neededDirectories[directory] = struct{}{}
			}
		}
	}
	directories := make([]string, 0, len(neededDirectories))
	for directory := range neededDirectories {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], "/")
		rightDepth := strings.Count(directories[right], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return directories[left] < directories[right]
	})
	transitions := make([]assemblyline.TargetTreeTransition, 0, len(directories)+len(assembly.Files))
	for _, directory := range directories {
		transitions = append(transitions, assemblyline.TargetTreeTransition{Kind: assemblyline.TargetTreeEnsureDirectory, Path: directory})
	}
	for _, file := range assembly.Files {
		kind := assemblyline.TargetTreeCreate
		if _, exists := presentFiles[file.Path]; exists {
			kind = assemblyline.TargetTreeReconcile
		}
		transitions = append(transitions, assemblyline.TargetTreeTransition{Kind: kind, Path: file.Path})
	}
	return transitions, nil
}

// requireDirectCodingTargetTreeLeafSource binds one accepted target-tree file
// leaf to one code-owned source job. The tree decides which artifact exists;
// code decides and validates every filesystem mutation. A declared leaf is
// never silently discarded because the current adapter lacks work for it.
func requireDirectCodingTargetTreeLeafSource(
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
		"target-tree path %q has no code-owned source job",
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
