package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

var retiredBulkFreshApplicationEntrypoints = map[string]struct{}{
	"generateProgramFragments":                {},
	"generateDirectCodingTypeScriptFragments": {},
	"runDirectCodingTypeScriptFragmentWave":   {},
	"stageProgram":                            {},
	"stageTypeScriptProgram":                  {},
}

func TestRetiredBulkFreshApplicationEntrypointsAreAbsentFromProduction(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(files, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse production source %s: %v", name, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, retired := retiredBulkFreshApplicationEntrypoints[identifier.Name]; retired {
				t.Errorf(
					"production retains retired bulk fresh-application entrypoint %s at %s",
					identifier.Name,
					files.Position(identifier.Pos()),
				)
			}
			return true
		})
	}
}

func TestFreshApplicationDriverEntersExactlyOneTaskLifecycle(t *testing.T) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "v3_coding_driver_plan.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := bulkAbsenceAssembleBody(t, parsed)
	counts := make(map[string]int)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		counts[bulkAbsenceCallName(call.Fun)]++
		return true
	})
	if counts["runDirectCodingApplicationTaskLifecycle"] != 1 {
		t.Fatalf("fresh driver task-lifecycle calls=%d", counts["runDirectCodingApplicationTaskLifecycle"])
	}
	for retired := range retiredBulkFreshApplicationEntrypoints {
		if counts[retired] != 0 {
			t.Fatalf("fresh driver still calls retired bulk entrypoint %s", retired)
		}
	}
}

func bulkAbsenceAssembleBody(t *testing.T, file *ast.File) *ast.BlockStmt {
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

func bulkAbsenceCallName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}
