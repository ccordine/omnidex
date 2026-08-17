package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingOrderAssemblyFilesByArtifactGraph derives the sole legal file
// materialization order from the already persisted relationship graph. Stable
// original order is retained only among leaves with no dependency between them.
func directCodingOrderAssemblyFilesByArtifactGraph(
	assembly directCodingAssembly,
	graph assemblyline.ArtifactGraph,
) (directCodingAssembly, error) {
	if err := assembly.validate(); err != nil {
		return directCodingAssembly{}, err
	}
	if err := graph.Validate(); err != nil {
		return directCodingAssembly{}, fmt.Errorf("artifact graph: %w", err)
	}
	artifactByID := make(map[string]assemblyline.ArtifactGraphArtifact, len(graph.Artifacts))
	for _, artifact := range graph.Artifacts {
		artifactByID[artifact.ID] = artifact
	}
	indexByPath := make(map[string]int, len(assembly.Files))
	for index, file := range assembly.Files {
		if _, exists := indexByPath[file.Path]; exists {
			return directCodingAssembly{}, fmt.Errorf("assembly repeats artifact path %q", file.Path)
		}
		indexByPath[file.Path] = index
	}
	prerequisites := make([]map[int]struct{}, len(assembly.Files))
	dependents := make([][]int, len(assembly.Files))
	for index := range prerequisites {
		prerequisites[index] = make(map[int]struct{})
	}
	for _, relation := range graph.Relations {
		if !artifactRelationCreatesPrerequisite(relation.Kind) {
			continue
		}
		dependentArtifact, dependentExists := artifactByID[relation.From]
		prerequisiteArtifact, prerequisiteExists := artifactByID[relation.To]
		if !dependentExists || !prerequisiteExists {
			return directCodingAssembly{}, fmt.Errorf("artifact graph relation %s has an unknown endpoint", relation.Kind)
		}
		dependent, dependentExists := indexByPath[dependentArtifact.Path]
		prerequisite, prerequisiteExists := indexByPath[prerequisiteArtifact.Path]
		if !dependentExists || !prerequisiteExists {
			return directCodingAssembly{}, fmt.Errorf(
				"artifact graph relation %s from %q to %q has no assembly files",
				relation.Kind, dependentArtifact.Path, prerequisiteArtifact.Path,
			)
		}
		if dependent == prerequisite {
			continue
		}
		if _, exists := prerequisites[dependent][prerequisite]; exists {
			continue
		}
		prerequisites[dependent][prerequisite] = struct{}{}
		dependents[prerequisite] = append(dependents[prerequisite], dependent)
	}

	ready := make([]int, 0, len(assembly.Files))
	for index, dependencies := range prerequisites {
		if len(dependencies) == 0 {
			ready = append(ready, index)
		}
	}
	ordered := make([]directCodingFileTask, 0, len(assembly.Files))
	for len(ready) > 0 {
		index := ready[0]
		ready = ready[1:]
		ordered = append(ordered, assembly.Files[index])
		for _, dependent := range dependents[index] {
			delete(prerequisites[dependent], index)
			if len(prerequisites[dependent]) == 0 {
				ready = insertDirectCodingReadyFileIndex(ready, dependent)
			}
		}
	}
	if len(ordered) != len(assembly.Files) {
		blocked := make([]string, 0)
		for index, dependencies := range prerequisites {
			if len(dependencies) > 0 {
				blocked = append(blocked, assembly.Files[index].Path)
			}
		}
		return directCodingAssembly{}, fmt.Errorf("artifact graph has a filesystem dependency cycle involving %v", blocked)
	}
	assembly.Files = ordered
	return assembly, nil
}

func insertDirectCodingReadyFileIndex(ready []int, value int) []int {
	index := 0
	for index < len(ready) && ready[index] < value {
		index++
	}
	ready = append(ready, 0)
	copy(ready[index+1:], ready[index:])
	ready[index] = value
	return ready
}
