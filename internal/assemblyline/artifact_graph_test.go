package assemblyline

import "testing"

func TestArtifactGraphRequiresCodeOwnedArtifactsAndRelations(t *testing.T) {
	graph := ArtifactGraph{
		Schema: ArtifactGraphSchemaV1,
		Artifacts: []ArtifactGraphArtifact{
			{ID: "artifact_001", Path: "src/Producer.ts", AdapterID: "typescript", Kind: TargetArtifactImplementation, Interfaces: []string{"export function Produce(): string"}},
			{ID: "artifact_002", Path: "src/Consumer.ts", AdapterID: "typescript", Kind: TargetArtifactImplementation},
		},
		Relations: []ArtifactGraphRelation{{From: "artifact_001", To: "artifact_002", Kind: ArtifactRelationProvides}},
	}
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	ordered := graph.Sorted()
	if len(ordered.Artifacts) != 2 || ordered.Artifacts[0].ID != "artifact_001" {
		t.Fatalf("sorted graph=%+v", ordered)
	}
}

func TestArtifactGraphRejectsUnprovedOrAmbiguousStructure(t *testing.T) {
	base := ArtifactGraph{
		Schema: ArtifactGraphSchemaV1,
		Artifacts: []ArtifactGraphArtifact{
			{ID: "artifact_001", Path: "src/One.ts", AdapterID: "typescript", Kind: TargetArtifactImplementation},
			{ID: "artifact_002", Path: "src/Two.ts", AdapterID: "typescript", Kind: TargetArtifactImplementation},
		},
		Relations: []ArtifactGraphRelation{{From: "artifact_001", To: "artifact_002", Kind: ArtifactRelationConsumes}},
	}
	for name, mutate := range map[string]func(*ArtifactGraph){
		"duplicate relation": func(graph *ArtifactGraph) {
			graph.Relations = append(graph.Relations, graph.Relations[0])
		},
		"unknown endpoint": func(graph *ArtifactGraph) {
			graph.Relations[0].To = "artifact_999"
		},
		"self relation": func(graph *ArtifactGraph) {
			graph.Relations[0].To = "artifact_001"
		},
		"path escape": func(graph *ArtifactGraph) {
			graph.Artifacts[0].Path = "../escape.ts"
		},
	} {
		t.Run(name, func(t *testing.T) {
			graph := base
			graph.Artifacts = append([]ArtifactGraphArtifact(nil), base.Artifacts...)
			graph.Relations = append([]ArtifactGraphRelation(nil), base.Relations...)
			mutate(&graph)
			if err := graph.Validate(); err == nil {
				t.Fatal("invalid graph was accepted")
			}
		})
	}
}
