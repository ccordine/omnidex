package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEveryProductionStepEventHasARegisteredGUIProjection(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join("..", "worker", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	registered := readProgressProjectionSource(t)
	fileSet := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isEmitStepEventCall(call.Fun) || len(call.Args) < 2 {
				return true
			}
			switch value := call.Args[1].(type) {
			case *ast.BasicLit:
				eventType, unquoteErr := strconv.Unquote(value.Value)
				if unquoteErr != nil {
					t.Errorf("%s has malformed event literal %s", path, value.Value)
					return true
				}
				if !strings.Contains(registered, `"`+eventType+`"`) {
					t.Errorf("production event %q from %s has no GUI projection", eventType, path)
				}
			default:
				rendered := sourceExpression(fileSet, value)
				if !registeredDynamicProgressEvent(rendered) {
					t.Errorf("production event emitter %s has unregistered dynamic identity %q", path, rendered)
				}
			}
			return true
		})
	}
}

func isEmitStepEventCall(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "emitStepEvent"
}

func registeredDynamicProgressEvent(value string) bool {
	for _, registered := range []string{
		`eventNamespace+"_portable_dispatched"`,
		`eventNamespace+"_worker_"+string(event.State)`,
		`repositoryVerificationAcceptanceEvent(scope)`,
	} {
		if value == registered {
			return true
		}
	}
	return false
}

func sourceExpression(fileSet *token.FileSet, expression ast.Expr) string {
	start := fileSet.Position(expression.Pos()).Offset
	end := fileSet.Position(expression.End()).Offset
	path := fileSet.Position(expression.Pos()).Filename
	raw, err := os.ReadFile(path)
	if err != nil || start < 0 || end < start || end > len(raw) {
		return "<unreadable>"
	}
	return strings.Join(strings.Fields(string(raw[start:end])), "")
}

func readProgressProjectionSource(t *testing.T) string {
	t.Helper()
	var source strings.Builder
	for _, path := range []string{
		"chat_job_progress_summary.go",
		"chat_job_progress_stations.go",
		"chat_job_progress_cognition.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
	}
	return source.String()
}
