package queue

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionDoesNotEquateWorkerAndCognitionPlanGenerations(t *testing.T) {
	roots := []string{"../", "../../cmd"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				binary, ok := node.(*ast.BinaryExpr)
				if !ok || binary.Op != token.EQL && binary.Op != token.NEQ {
					return true
				}
				left, right := renderPlanGenerationExpression(binary.X), renderPlanGenerationExpression(binary.Y)
				if workerPlanGenerationPair(left, right) || workerPlanGenerationPair(right, left) {
					t.Errorf("%s equates worker/job generation with cognition plan generation: %s %s %s",
						path, left, binary.Op, right)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		"cognition_runtime_snapshot_authority.go", "cognition_terminal_validation.go",
		"task_generation_retirement.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "obligations.created_"+"generation=$3") {
			t.Fatalf("%s still uses plan generation as job lifecycle authority", path)
		}
	}
}

func renderPlanGenerationExpression(expression ast.Expr) string {
	var buffer bytes.Buffer
	_ = format.Node(&buffer, token.NewFileSet(), expression)
	return buffer.String()
}

func workerPlanGenerationPair(plan, worker string) bool {
	isPlan := strings.Contains(plan, "CreatedGeneration") ||
		strings.Contains(plan, "ObligationGraph.Generation") ||
		strings.Contains(plan, "Graph.Generation")
	isWorker := strings.Contains(worker, "Attempt().Generation") ||
		strings.Contains(worker, "Attempt.Generation") ||
		strings.Contains(worker, "Actor.Generation")
	return isPlan && isWorker
}
