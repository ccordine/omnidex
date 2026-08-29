package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeLeafRequiresOneContentJob(t *testing.T) {
	program := directCodingProgram{StructureTransitions: []assemblyline.TargetTreeTransition{{Kind: assemblyline.TargetTreeCreate, Path: "src/Counter.tsx"}}}
	_, err := directCodingAssemblyFromProgram(program)
	if err == nil || !strings.Contains(err.Error(), "has no code-owned source job") {
		t.Fatalf("error=%v", err)
	}
}

func TestTargetTreeAssemblyPreservesCodeDerivedDirectoryLeafOrder(t *testing.T) {
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: []directCodingFileTask{
			{Path: "package.json", Content: "{}\n"},
			{Path: "src/components/Counter.ts", Content: "export {};\n"},
			{Path: "src/runtime.ts", Content: "export {};\n"},
		},
		Generated: map[string]string{},
		StructureTransitions: []assemblyline.TargetTreeTransition{
			{Kind: assemblyline.TargetTreeCreate, Path: "src/components/Counter.ts"},
		},
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := directCodingAssemblyFilesystemTransitions(
		nil, nil, program.StructureTransitions, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join([]string{transitions[0].Path, transitions[1].Path}, ","); got != "src,src/components" {
		t.Fatalf("transitions=%+v", transitions)
	}
	if len(assembly.Files) != 3 || assembly.Files[1].Path != "src/components/Counter.ts" {
		t.Fatalf("files=%+v", assembly.Files)
	}
	if got := strings.Join([]string{transitions[2].Path, transitions[3].Path, transitions[4].Path}, ","); got != "package.json,src/components/Counter.ts,src/runtime.ts" {
		t.Fatalf("file transitions=%+v", transitions)
	}
}

func TestAdapterBaselineFilesBecomeCodeOwnedTreeLeaves(t *testing.T) {
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: []directCodingFileTask{
			{Path: "package.json", Content: "{}\n"},
			{Path: "src/main.tsx", Content: "export {};\n"},
		},
		Generated: map[string]string{},
		StructureTransitions: []assemblyline.TargetTreeTransition{
			{Kind: assemblyline.TargetTreeCreate, Path: "src/components/Counter.tsx"},
			{Kind: assemblyline.TargetTreeCreate, Path: "src/components/Counter.test.tsx"},
		},
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{ID: "counter", Path: "src/components/Counter.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{{ID: "counter.block", Static: "export {};", API: "module marker"}}},
			{ID: "counter_test", Path: "src/components/Counter.test.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{{ID: "counter_test.block", Static: "export {};", API: "test module marker"}}},
			{ID: "app", Path: "src/App.tsx", AdapterID: "typescript_react", Blocks: []assemblyline.SourceBlock{{ID: "app.block", Static: "export {};", API: "application module marker"}}},
		}},
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := directCodingAssemblyFilesystemTransitions(
		nil, nil, program.StructureTransitions, assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, transition := range transitions {
		if transition.Kind != assemblyline.TargetTreeEnsureDirectory {
			files = append(files, transition.Path)
		}
	}
	if got := strings.Join(files, ","); got != "package.json,src/main.tsx,src/components/Counter.tsx,src/components/Counter.test.tsx,src/App.tsx" {
		t.Fatalf("file leaves=%q", got)
	}
}

func TestTargetTreeDeleteBecomesOneCodeOwnedAssemblyAndFilesystemLeaf(t *testing.T) {
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: []directCodingFileTask{{Path: "src/current.ts", Content: "export {};\n"}},
		Generated:   map[string]string{},
		StructureTransitions: []assemblyline.TargetTreeTransition{
			{Kind: assemblyline.TargetTreeDelete, Path: "src/obsolete.ts"},
			{Kind: assemblyline.TargetTreeReconcile, Path: "src/current.ts"},
		},
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if len(assembly.DeletePaths) != 1 || assembly.DeletePaths[0] != "src/obsolete.ts" {
		t.Fatalf("delete paths=%v", assembly.DeletePaths)
	}
	transitions, err := directCodingAssemblyFilesystemTransitions(
		[]string{"src/current.ts", "src/obsolete.ts"},
		[]string{"src"},
		program.StructureTransitions,
		assembly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 ||
		transitions[0] != (assemblyline.TargetTreeTransition{
			Kind: assemblyline.TargetTreeReconcile, Path: "src/current.ts",
		}) ||
		transitions[1] != (assemblyline.TargetTreeTransition{
			Kind: assemblyline.TargetTreeDelete, Path: "src/obsolete.ts",
		}) {
		t.Fatalf("transitions=%+v", transitions)
	}
}

func TestTargetTreeDeleteCannotAlsoHaveCompiledSource(t *testing.T) {
	program := directCodingProgram{
		StackID: genericTypeScriptBrowserAdapter, VersionProfileID: typeScriptBrowserVersionProfileV1,
		StaticFiles: []directCodingFileTask{{Path: "src/ambiguous.ts", Content: "export {};\n"}},
		Generated:   map[string]string{},
		StructureTransitions: []assemblyline.TargetTreeTransition{{
			Kind: assemblyline.TargetTreeDelete, Path: "src/ambiguous.ts",
		}},
	}
	if _, err := directCodingAssemblyFromProgram(program); err == nil ||
		!strings.Contains(err.Error(), "also present in the compiled program") {
		t.Fatalf("error=%v", err)
	}
}
