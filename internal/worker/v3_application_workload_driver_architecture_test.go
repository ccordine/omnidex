package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestFreshApplicationDriverFreezesWorkloadBeforeCompilationAndGeneration(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("v3_coding_driver_plan.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "v3_coding_driver_plan.go", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := directCodingAssembleBody(t, file)
	calls := make([]string, 0)
	var compileCall *ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := workloadCallName(call.Fun)
		calls = append(calls, name)
		if name == "compileDirectCodingProgram" {
			compileCall = call
		}
		return true
	})

	interpreter := uniqueWorkloadCallIndex(t, calls, "runDirectCodingApplicationInterpreter")
	freeze := uniqueWorkloadCallIndex(t, calls, "resolveDirectCodingApplicationWorkload")
	capabilities := uniqueWorkloadCallIndex(t, calls, "deriveRequirementCapabilities")
	compile := uniqueWorkloadCallIndex(t, calls, "compileDirectCodingProgram")
	execute := uniqueWorkloadCallIndex(t, calls, "runDirectCodingApplicationTaskLifecycle")
	if !(interpreter < freeze && freeze < capabilities && capabilities < compile && compile < execute) {
		t.Fatalf("fresh path call order=%v", calls)
	}
	bindings := uniqueWorkloadCallIndex(t, calls, "deriveDirectCodingTargetTreeBindings")
	assembly := uniqueWorkloadCallIndex(t, calls, "directCodingAssemblyFromProgram")
	graph := uniqueWorkloadCallIndex(t, calls, "directCodingArtifactGraphFromProgram")
	record := uniqueWorkloadCallIndex(t, calls, "RecordArtifactGraph")
	planLeaves := uniqueWorkloadCallIndex(t, calls, "PlanTreeTransitionsWithArtifactGraph")
	if !(execute < assembly && assembly < graph && graph < record && record < planLeaves) {
		t.Fatalf("artifact graph was not persisted before filesystem leaf planning: %v", calls)
	}
	if !(freeze < bindings && bindings < compile) {
		t.Fatalf("code-derived forced tree binding order=%v", calls)
	}
	for _, forbidden := range []string{"generateProgramFragments", "stageProgram", "resolveDirectCodingFileContents"} {
		if slices.Contains(calls, forbidden) {
			t.Fatalf("fresh path bypasses the task lifecycle through %s: %v", forbidden, calls)
		}
	}
	if compileCall == nil || !callHasNamedArgument(compileCall, "workload") ||
		!callHasNamedArgument(compileCall, "capabilit") {
		t.Fatal("deterministic compiler did not receive the frozen workload and separate capability graph")
	}
}

func directCodingAssembleBody(t *testing.T, file *ast.File) *ast.BlockStmt {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "Assemble" && function.Recv != nil {
			return function.Body
		}
	}
	t.Fatal("directCodingSession.Assemble is missing")
	return nil
}

func workloadCallName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func uniqueWorkloadCallIndex(t *testing.T, calls []string, target string) int {
	t.Helper()
	first := -1
	count := 0
	for index, call := range calls {
		if call == target {
			first = index
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%s call count=%d in %v", target, count, calls)
	}
	return first
}

func callHasNamedArgument(call *ast.CallExpr, fragment string) bool {
	found := false
	for _, argument := range call.Args {
		ast.Inspect(argument, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && strings.Contains(strings.ToLower(identifier.Name), fragment) {
				found = true
			}
			return !found
		})
	}
	return found
}
