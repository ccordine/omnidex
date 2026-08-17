package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeLeafRequiresOneContentJob(t *testing.T) {
	program := directCodingProgram{StructureTransitions: []assemblyline.TargetTreeTransition{{Kind: assemblyline.TargetTreeCreate, Path: "src/Counter.tsx"}}}
	_, err := directCodingAssemblyFromProgram(program)
	if err == nil || !strings.Contains(err.Error(), "has no code-owned file-content job") {
		t.Fatalf("error=%v", err)
	}
}

func TestTargetTreeAssemblyPreservesCodeDerivedDirectoryLeafOrder(t *testing.T) {
	program := directCodingProgram{
		StaticFiles: []directCodingFileTask{{Path: "src/components/Counter.ts", Content: "export {};\n"}},
		Generated:   map[string]string{},
		StructureTransitions: []assemblyline.TargetTreeTransition{
			{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src"},
			{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src/components"},
			{Kind: assemblyline.TargetTreeCreate, Path: "src/components/Counter.ts"},
		},
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(assembly.Directories, ","); got != "src,src/components" {
		t.Fatalf("directories=%q", got)
	}
	if len(assembly.Files) != 1 || assembly.Files[0].Path != "src/components/Counter.ts" {
		t.Fatalf("files=%+v", assembly.Files)
	}
}
