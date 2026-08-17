package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestDirectCodingArtifactGraphProjectsVerifiedInterfacesAndRelations(t *testing.T) {
	program, assembly := directCodingArtifactGraphFixture(t)
	graph, err := directCodingArtifactGraphFromProgram(program, assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.Validate(); err != nil {
		t.Fatal(err)
	}
	counter := artifactGraphArtifactByPath(t, graph, "src/components/Counter.tsx")
	if !strings.Contains(strings.Join(counter.Interfaces, "\n"), "function Counter") {
		t.Fatalf("counter interfaces=%q", counter.Interfaces)
	}
	assertArtifactGraphRelation(t, graph, "src/App.tsx", "src/components/Counter.tsx", assemblyline.ArtifactRelationDependsOn)
	assertArtifactGraphRelation(t, graph, "src/components/Counter.test.tsx", "src/components/Counter.tsx", assemblyline.ArtifactRelationVerifies)
	assertArtifactGraphRelation(t, graph, "src/main.tsx", "src/App.tsx", assemblyline.ArtifactRelationComposes)
}

func TestDirectCodingArtifactGraphPersistsAndOrdersFilesystemLeaves(t *testing.T) {
	program, assembly := directCodingArtifactGraphFixture(t)
	graph, err := directCodingArtifactGraphFromProgram(program, assembly)
	if err != nil {
		t.Fatal(err)
	}
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build a browser workspace.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles: map[string]assemblyline.TargetTreeTransition{}, treeDirs: map[string]assemblyline.TargetTreeTransition{},
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.RecordArtifactGraph(graph); err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		if err := coordinator.Begin(task.ID); err != nil {
			t.Fatal(err)
		}
		if err := coordinator.CompleteTask(task.ID, map[string]string{
			"feature": "export {};", "acceptance": "export {};",
		}); err != nil {
			t.Fatal(err)
		}
	}
	assembly, err = directCodingOrderAssemblyFilesByArtifactGraph(assembly, graph)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := directCodingAssemblyFilesystemTransitions(nil, nil, nil, assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.PlanTreeTransitionsWithArtifactGraph(transitions, graph); err != nil {
		t.Fatal(err)
	}
	ledger := store.ledger.MaterializedState()
	if entry, exists := store.ledger.Entry(directCodingArtifactGraphEntryID); !exists || entry.Kind != taskstate.EntryFact || entry.ScopeNodeID != coordinator.objectiveID {
		t.Fatalf("artifact graph ledger fact=%+v exists=%t", entry, exists)
	}
	assertTreeArtifactDependency(t, coordinator, ledger, "src/App.tsx", "src/components/Counter.tsx")
	assertTreeArtifactDependency(t, coordinator, ledger, "src/components/Counter.test.tsx", "src/components/Counter.tsx")
	assertTreeArtifactDependency(t, coordinator, ledger, "src/main.tsx", "src/App.tsx")
	for _, path := range []string{"src/App.tsx", "src/components/Counter.test.tsx", "src/main.tsx"} {
		if node := taskNode(t, ledger, artifactTreeTaskID(t, coordinator, path)); node.Status != taskstate.NodePending {
			t.Fatalf("dependent tree leaf %q was promoted before its graph prerequisite: %+v", path, node)
		}
	}
	for _, transition := range transitions {
		if transition.Kind != assemblyline.TargetTreeEnsureDirectory {
			continue
		}
		if err := coordinator.BeginTreeTransition(transition); err != nil {
			t.Fatalf("begin directory %s: %v", transition.Path, err)
		}
		if err := coordinator.CompleteTreeTransition(transition, "verified "+transition.Path); err != nil {
			t.Fatalf("complete directory %s: %v", transition.Path, err)
		}
	}
	for _, file := range assembly.Files {
		if err := coordinator.BeginTreeFile(file.Path); err != nil {
			t.Fatalf("begin file %s: %v", file.Path, err)
		}
		if err := coordinator.CompleteTreeFile(file.Path, "verified "+file.Path); err != nil {
			t.Fatalf("complete file %s: %v", file.Path, err)
		}
	}
	ledger = store.ledger.MaterializedState()
	for _, transition := range transitions {
		id := artifactTreeTaskIDForTransition(t, coordinator, transition)
		if node := taskNode(t, ledger, id); node.Status != taskstate.NodeDone {
			t.Fatalf("tree leaf %q did not complete through graph order: %+v", transition.Path, node)
		}
	}
}

func TestDirectCodingArtifactGraphOrdersFilesBeforeDependentLeaves(t *testing.T) {
	program, assembly := directCodingArtifactGraphFixture(t)
	graph, err := directCodingArtifactGraphFromProgram(program, assembly)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := directCodingOrderAssemblyFilesByArtifactGraph(assembly, graph)
	if err != nil {
		t.Fatal(err)
	}
	indices := make(map[string]int, len(ordered.Files))
	for index, file := range ordered.Files {
		indices[file.Path] = index
	}
	for dependent, prerequisite := range map[string]string{
		"src/App.tsx":                     "src/components/Counter.tsx",
		"src/components/Counter.test.tsx": "src/components/Counter.tsx",
		"src/main.tsx":                    "src/App.tsx",
	} {
		if indices[dependent] <= indices[prerequisite] {
			t.Fatalf("ordered files violate %q after %q: %+v", dependent, prerequisite, ordered.Files)
		}
	}
}

func TestDirectCodingArtifactGraphRejectsFilesystemDependencyCycle(t *testing.T) {
	program, assembly := directCodingArtifactGraphFixture(t)
	graph, err := directCodingArtifactGraphFromProgram(program, assembly)
	if err != nil {
		t.Fatal(err)
	}
	app := artifactGraphArtifactByPath(t, graph, "src/App.tsx")
	counter := artifactGraphArtifactByPath(t, graph, "src/components/Counter.tsx")
	graph.Relations = append(graph.Relations, assemblyline.ArtifactGraphRelation{
		From: counter.ID, To: app.ID, Kind: assemblyline.ArtifactRelationDependsOn,
	})
	if _, err := directCodingOrderAssemblyFilesByArtifactGraph(assembly, graph); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("cycle error=%v", err)
	}
}

func directCodingArtifactGraphFixture(t *testing.T) (directCodingProgram, directCodingAssembly) {
	t.Helper()
	program := directCodingProgram{TypeScript: assemblyline.TypeScriptBlueprint{Documents: []assemblyline.TypeScriptDocument{
		{ID: "counter", Path: "src/components/Counter.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "counter.render", Static: "export function Counter(): null { return null; }", API: "function Counter(): null",
		}}},
		{ID: "counter_test", Path: "src/components/Counter.test.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "counter.verify", Static: "export function VerifyCounter(): null { return null; }", API: "function VerifyCounter(): null", DependsOn: []string{"counter.render"},
		}}},
		{ID: "app", Path: "src/App.tsx", Blocks: []assemblyline.TypeScriptBlock{{
			ID: "app.render", Static: "export function App(): null { return null; }", API: "function App(): null", DependsOn: []string{"counter.render"},
		}}},
	}}}
	assembly := directCodingAssembly{Files: []directCodingFileTask{
		{Path: ".gitignore", Content: "dist\n"},
		{Path: "package.json", Content: "{}\n"},
		{Path: "src/main.tsx", Content: "export {};\n"},
		{Path: "src/components/Counter.tsx", Content: "export function Counter(): null { return null; }\n"},
		{Path: "src/components/Counter.test.tsx", Content: "export function VerifyCounter(): null { return null; }\n"},
		{Path: "src/App.tsx", Content: "export function App(): null { return null; }\n"},
	}}
	if err := assembly.normalize(); err != nil {
		t.Fatal(err)
	}
	return program, assembly
}

func artifactGraphArtifactByPath(t *testing.T, graph assemblyline.ArtifactGraph, path string) assemblyline.ArtifactGraphArtifact {
	t.Helper()
	for _, artifact := range graph.Artifacts {
		if artifact.Path == path {
			return artifact
		}
	}
	t.Fatalf("graph lacks artifact %q", path)
	return assemblyline.ArtifactGraphArtifact{}
}

func assertArtifactGraphRelation(t *testing.T, graph assemblyline.ArtifactGraph, fromPath, toPath string, kind assemblyline.ArtifactRelationKind) {
	t.Helper()
	from := artifactGraphArtifactByPath(t, graph, fromPath)
	to := artifactGraphArtifactByPath(t, graph, toPath)
	for _, relation := range graph.Relations {
		if relation.From == from.ID && relation.To == to.ID && relation.Kind == kind {
			return
		}
	}
	t.Fatalf("graph lacks %s relation from %q to %q: %+v", kind, fromPath, toPath, graph.Relations)
}

func assertTreeArtifactDependency(t *testing.T, coordinator *directCodingTaskCognition, ledger taskstate.MaterializedState, dependentPath, prerequisitePath string) {
	t.Helper()
	dependent := coordinator.treeFiles[dependentPath]
	prerequisite := coordinator.treeFiles[prerequisitePath]
	dependentKey, err := directCodingTreeTaskKey(dependent)
	if err != nil {
		t.Fatal(err)
	}
	prerequisiteKey, err := directCodingTreeTaskKey(prerequisite)
	if err != nil {
		t.Fatal(err)
	}
	if !taskCognitionHasEdge(ledger, taskstate.EdgeDependsOn, coordinator.treeTaskIDs[dependentKey], coordinator.treeTaskIDs[prerequisiteKey]) {
		t.Fatalf("tree dependency %q -> %q is absent", dependentPath, prerequisitePath)
	}
}

func artifactTreeTaskID(t *testing.T, coordinator *directCodingTaskCognition, path string) taskstate.NodeID {
	t.Helper()
	transition, exists := coordinator.treeFiles[path]
	if !exists {
		t.Fatalf("tree does not contain file %q", path)
	}
	return artifactTreeTaskIDForTransition(t, coordinator, transition)
}

func artifactTreeTaskIDForTransition(t *testing.T, coordinator *directCodingTaskCognition, transition assemblyline.TargetTreeTransition) taskstate.NodeID {
	t.Helper()
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		t.Fatal(err)
	}
	return coordinator.treeTaskIDs[key]
}
