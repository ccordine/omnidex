package repository

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

type GoPackagePlacement struct {
	ArtifactID string
	Name       string
	Directory  string
}

// UniqueGoPackagePlacement returns a placement only when the exact analysis
// exposes one legal Go package. Ambiguous repositories require a separate
// opaque candidate-selection gap; callers must never guess from paths/names.
func UniqueGoPackagePlacement(snapshot Snapshot, analysis Analysis) (GoPackagePlacement, error) {
	if err := analysis.Validate(snapshot); err != nil {
		return GoPackagePlacement{}, fmt.Errorf("resolve unique Go package placement: %w", err)
	}
	ids := make([]string, 0)
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind != "go_package" || artifact.FileID != "" {
			continue
		}
		if _, err := ResolveGoPackagePlacement(snapshot, analysis, artifact.ID); err == nil {
			ids = append(ids, artifact.ID)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return GoPackagePlacement{}, fmt.Errorf("repository has no exact Go package placement")
	}
	if len(ids) != 1 {
		return GoPackagePlacement{}, fmt.Errorf("repository Go package placement is ambiguous across %d opaque candidates", len(ids))
	}
	return ResolveGoPackagePlacement(snapshot, analysis, ids[0])
}

// GoPackagePlacementForSymbols proves that every selected declaration belongs
// to one and only one indexed package, without exposing its path to a model.
func GoPackagePlacementForSymbols(
	snapshot Snapshot,
	analysis Analysis,
	symbolIDs []string,
) (GoPackagePlacement, error) {
	if err := analysis.Validate(snapshot); err != nil {
		return GoPackagePlacement{}, fmt.Errorf("resolve Go declaration package: %w", err)
	}
	if len(symbolIDs) == 0 {
		return GoPackagePlacement{}, fmt.Errorf("resolve Go declaration package requires exact symbol authority")
	}
	wanted := make(map[string]struct{}, len(symbolIDs))
	for _, id := range symbolIDs {
		if id == "" {
			return GoPackagePlacement{}, fmt.Errorf("resolve Go declaration package contains empty symbol authority")
		}
		wanted[id] = struct{}{}
	}
	owners := make([]string, 0)
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind != "go_package" || artifact.FileID != "" {
			continue
		}
		contained := make(map[string]struct{})
		for _, edge := range analysis.Edges {
			if edge.Kind == "contains" && edge.FromID == artifact.ID {
				contained[edge.ToID] = struct{}{}
			}
		}
		ownsAll := true
		for symbolID := range wanted {
			if _, exists := contained[symbolID]; !exists {
				ownsAll = false
				break
			}
		}
		if ownsAll {
			owners = append(owners, artifact.ID)
		}
	}
	sort.Strings(owners)
	if len(owners) != 1 {
		return GoPackagePlacement{}, fmt.Errorf("selected Go declarations have %d exact package owners", len(owners))
	}
	return ResolveGoPackagePlacement(snapshot, analysis, owners[0])
}

// ResolveGoPackagePlacement derives one physical package directory entirely
// from exact index authority. Callers may select only an opaque artifact ID;
// paths never need to cross a semantic-model boundary.
func ResolveGoPackagePlacement(
	snapshot Snapshot,
	analysis Analysis,
	artifactID string,
) (GoPackagePlacement, error) {
	if err := analysis.Validate(snapshot); err != nil {
		return GoPackagePlacement{}, fmt.Errorf("resolve Go package placement: %w", err)
	}
	if strings.TrimSpace(artifactID) == "" {
		return GoPackagePlacement{}, fmt.Errorf("resolve Go package placement requires one opaque artifact ID")
	}
	var artifact Artifact
	for _, candidate := range analysis.Artifacts {
		if candidate.ID == artifactID {
			artifact = candidate
			break
		}
	}
	if artifact.ID == "" || artifact.Kind != "go_package" || artifact.FileID != "" {
		return GoPackagePlacement{}, fmt.Errorf("Go package artifact %q is absent or unsupported", artifactID)
	}
	packageName := artifact.Detail["package_name"]
	if packageName == "" || packageName != strings.TrimSpace(packageName) {
		return GoPackagePlacement{}, fmt.Errorf("Go package artifact %q has no exact package name", artifactID)
	}
	files := make(map[string]File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	symbols := make(map[string]Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	directorySet := make(map[string]struct{})
	for _, edge := range analysis.Edges {
		if edge.Kind != "contains" || edge.FromID != artifactID {
			continue
		}
		symbol, exists := symbols[edge.ToID]
		if !exists {
			return GoPackagePlacement{}, fmt.Errorf("Go package artifact references unknown symbol %q", edge.ToID)
		}
		file, exists := files[symbol.FileID]
		if !exists || file.Kind != EntryRegular || file.Language != "go" {
			return GoPackagePlacement{}, fmt.Errorf("Go package artifact references unsupported source authority")
		}
		directorySet[path.Dir(file.Path)] = struct{}{}
	}
	directories := make([]string, 0, len(directorySet))
	for directory := range directorySet {
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	if len(directories) == 0 {
		return GoPackagePlacement{}, fmt.Errorf("Go package artifact has no exact indexed source member")
	}
	if len(directories) != 1 {
		return GoPackagePlacement{}, fmt.Errorf("Go package placement is ambiguous across %d directories", len(directories))
	}
	return GoPackagePlacement{ArtifactID: artifactID, Name: packageName, Directory: directories[0]}, nil
}
