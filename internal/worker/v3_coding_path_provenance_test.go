package worker

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestDirectCodingPathProvenanceExtensionIsExactAndDeduplicated(t *testing.T) {
	base, err := modelcontext.NewArtifactIdentityProvenance([]string{"existing.go"})
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := extendDirectCodingPathProvenance(
		base, "src/Feature001.php", "existing.go", "Dockerfile",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Dockerfile", "existing.go", "src/Feature001.php"}
	if got := provenance.Paths(); !slices.Equal(got, want) {
		t.Fatalf("extended provenance=%v want=%v", got, want)
	}
	if _, err := extendDirectCodingPathProvenance(base, "../escape.go"); err == nil {
		t.Fatal("provenance extension accepted a non-normalized path")
	}
}

func TestPlannedTreeAndProgramPathsReachFragmentCandidateBoundary(t *testing.T) {
	session := &directCodingSession{}
	target := assemblyline.TargetTree{Paths: []string{
		"src/Feature001.php", "tests/Feature001Test.php",
	}}
	if err := session.bindDirectCodingTargetTreePathProvenance(target); err != nil {
		t.Fatal(err)
	}
	program := directCodingProgram{
		TargetTree: target,
		Source: assemblyline.SourceBlueprint{Documents: []assemblyline.SourceDocument{
			{Path: "runtime.go"},
		}},
		StaticFiles: []directCodingFileTask{{Path: "Dockerfile", Content: "FROM scratch\n"}},
	}
	if err := session.bindDirectCodingProgramPathProvenance(program); err != nil {
		t.Fatal(err)
	}

	for _, artifactName := range []string{"Feature001.php", "runtime.go", "Dockerfile"} {
		artifactName := artifactName
		t.Run(artifactName, func(t *testing.T) {
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1,
				PathProvenance: session.pathProvenance,
				Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
					return `function artifactLabel() { return "` + artifactName + `"; }`, nil
				}),
			}
			_, err := runDirectCodingLanguageFragmentWorker(
				runtime, "fragment-model", directCodingLanguageGenerationJob{
					Subject: "opaque-block",
					Input: assemblyline.FragmentGenerationInput{
						Language: "javascript", Dialect: "ECMAScript 2022", Signature: "function artifactLabel()",
						Behavior: "Return one stable semantic label.",
					},
					Project: assemblyline.ProjectJavaScriptFragment,
					Validate: func(_ assemblyline.FragmentGenerationInput, candidate string) (string, error) {
						return candidate, nil
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
				t.Fatalf("planned artifact %q candidate error=%v", artifactName, err)
			}
		})
	}
}

func TestDesiredRepositoryBindsTargetProvenanceBeforeGenerationRuntime(t *testing.T) {
	raw, err := os.ReadFile("v3_repository_desired_generation.go")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "v3_repository_desired_generation.go", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "generateDesiredRepositoryDeclarations" {
			body = function.Body
			break
		}
	}
	if body == nil {
		t.Fatal("desired repository generation entrypoint is missing")
	}
	calls := make([]string, 0)
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			calls = append(calls, workloadCallName(call.Fun))
		}
		return true
	})
	targets := uniqueWorkloadCallIndex(t, calls, "desiredRepositoryTargetPaths")
	bind := uniqueWorkloadCallIndex(t, calls, "extendPathProvenance")
	runtime := uniqueWorkloadCallIndex(t, calls, "directCodingWorkerRuntime")
	generation := uniqueWorkloadCallIndex(t, calls, "runDirectCodingGoFragmentGenerationWorker")
	if !(targets < bind && bind < runtime && runtime < generation) {
		t.Fatalf("desired repository provenance order=%v", calls)
	}
}
