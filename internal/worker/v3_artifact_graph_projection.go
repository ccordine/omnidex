package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingArtifactGraphFromProgram projects already assembled source into
// the normalized task-local graph. It is intentionally model-free: paths,
// adapters, exported surfaces, and cross-document dependencies are all known
// after code has accepted the generated blocks and composed the program.
func directCodingArtifactGraphFromProgram(
	program directCodingProgram,
	assembly directCodingAssembly,
) (assemblyline.ArtifactGraph, error) {
	if err := assembly.validate(); err != nil {
		return assemblyline.ArtifactGraph{}, err
	}
	artifacts := make([]assemblyline.ArtifactGraphArtifact, 0, len(assembly.Files))
	byPath := make(map[string]assemblyline.ArtifactGraphArtifact, len(assembly.Files))
	for _, file := range assembly.Files {
		adapter, kind, err := directCodingArtifactAdapterForPath(file.Path)
		if err != nil {
			return assemblyline.ArtifactGraph{}, fmt.Errorf("select artifact adapter for %q: %w", file.Path, err)
		}
		artifact := assemblyline.ArtifactGraphArtifact{
			ID:        directCodingArtifactGraphID(file.Path),
			Path:      file.Path,
			AdapterID: adapter.ID,
			Kind:      kind,
		}
		if _, exists := byPath[artifact.Path]; exists {
			return assemblyline.ArtifactGraph{}, fmt.Errorf("artifact graph repeats assembled path %q", artifact.Path)
		}
		artifacts = append(artifacts, artifact)
		byPath[artifact.Path] = artifact
	}

	blockDocuments := make(map[string]string)
	for _, document := range program.TypeScript.Documents {
		artifact, exists := byPath[document.Path]
		if !exists {
			return assemblyline.ArtifactGraph{}, fmt.Errorf("assembled graph lacks TypeScript document %q", document.Path)
		}
		for _, block := range document.Blocks {
			if previous, exists := blockDocuments[block.ID]; exists {
				return assemblyline.ArtifactGraph{}, fmt.Errorf("artifact graph block %q belongs to both %q and %q", block.ID, previous, document.Path)
			}
			blockDocuments[block.ID] = document.Path
			if block.API != "" {
				artifact.Interfaces = append(artifact.Interfaces, block.API)
			}
		}
		byPath[document.Path] = artifact
	}
	for index := range artifacts {
		artifacts[index] = byPath[artifacts[index].Path]
	}

	relations := make(map[string]assemblyline.ArtifactGraphRelation)
	addRelation := func(fromPath, toPath string, kind assemblyline.ArtifactRelationKind) error {
		from, fromExists := byPath[fromPath]
		to, toExists := byPath[toPath]
		if !fromExists || !toExists {
			return fmt.Errorf("artifact graph relation %s requires assembled paths %q and %q", kind, fromPath, toPath)
		}
		if from.ID == to.ID {
			return nil
		}
		relation := assemblyline.ArtifactGraphRelation{From: from.ID, To: to.ID, Kind: kind}
		key := string(kind) + "\x00" + relation.From + "\x00" + relation.To
		relations[key] = relation
		return nil
	}
	for _, document := range program.TypeScript.Documents {
		consumer, exists := byPath[document.Path]
		if !exists {
			return assemblyline.ArtifactGraph{}, fmt.Errorf("artifact graph lacks consumer document %q", document.Path)
		}
		for _, block := range document.Blocks {
			for _, dependency := range block.DependsOn {
				providerPath, exists := blockDocuments[dependency]
				if !exists || providerPath == document.Path {
					continue
				}
				kind := assemblyline.ArtifactRelationDependsOn
				if consumer.Kind == assemblyline.TargetArtifactVerification {
					kind = assemblyline.ArtifactRelationVerifies
				}
				if err := addRelation(document.Path, providerPath, kind); err != nil {
					return assemblyline.ArtifactGraph{}, err
				}
				if err := addRelation(providerPath, document.Path, assemblyline.ArtifactRelationProvides); err != nil {
					return assemblyline.ArtifactGraph{}, err
				}
			}
		}
	}
	// This baseline composition is fixed by the selected browser adapter, not
	// inferred from model prose or a source-text heuristic.
	if _, mainExists := byPath["src/main.tsx"]; mainExists {
		if _, appExists := byPath["src/App.tsx"]; appExists {
			if err := addRelation("src/main.tsx", "src/App.tsx", assemblyline.ArtifactRelationComposes); err != nil {
				return assemblyline.ArtifactGraph{}, err
			}
			if err := addRelation("src/App.tsx", "src/main.tsx", assemblyline.ArtifactRelationProvides); err != nil {
				return assemblyline.ArtifactGraph{}, err
			}
		}
	}

	orderedRelations := make([]assemblyline.ArtifactGraphRelation, 0, len(relations))
	for _, relation := range relations {
		orderedRelations = append(orderedRelations, relation)
	}
	sort.Slice(orderedRelations, func(left, right int) bool {
		if orderedRelations[left].From != orderedRelations[right].From {
			return orderedRelations[left].From < orderedRelations[right].From
		}
		if orderedRelations[left].To != orderedRelations[right].To {
			return orderedRelations[left].To < orderedRelations[right].To
		}
		return orderedRelations[left].Kind < orderedRelations[right].Kind
	})
	graph := assemblyline.ArtifactGraph{
		Schema: assemblyline.ArtifactGraphSchemaV1, Artifacts: artifacts, Relations: orderedRelations,
	}.Sorted()
	if err := graph.Validate(); err != nil {
		return assemblyline.ArtifactGraph{}, fmt.Errorf("validate code-owned artifact graph: %w", err)
	}
	return graph, nil
}

func directCodingArtifactGraphID(path string) string {
	return "artifact-" + directCodingDigest(path)
}
