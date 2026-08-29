package queue

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

func TestLegacyQueueCognitionClusterIsAbsent(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" {
			continue
		}
		if strings.HasPrefix(name, "cognition_") {
			t.Errorf("legacy queue cognition source remains: %s", name)
		}

		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s imports: %v", name, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("decode %s import %q: %v", name, spec.Path.Value, err)
			}
			if strings.HasPrefix(importPath, "github.com/gryph/omnidex/internal/cognition") {
				t.Errorf("%s retains legacy cognition dependency %q", name, importPath)
			}
		}
	}
}

func TestJobLifecycleHasNoLegacyCognitionDependency(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"repository_cancel.go", "repository_replan.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("decode %s import %q: %v", path, spec.Path.Value, err)
			}
			if strings.HasPrefix(importPath, "github.com/gryph/omnidex/internal/cognition") {
				t.Fatalf("%s retains legacy cognition dependency %q", path, importPath)
			}
		}

		file, err = parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			identifier, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			switch identifier.Name {
			case "requireCognitionLifecycleSealSetReplayTx",
				"retireCognitionEpisodesForLifecycleTx",
				"supersedeCurrentCognitionObligationsTx":
				t.Errorf("%s retains legacy cognition lifecycle call %s", path, identifier.Name)
			}
			return true
		})
	}
}

func TestLegacyTaskGenerationRetirementSourceIsAbsent(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("task_generation_retirement.go"); err == nil {
		t.Fatal("legacy task-generation cognition retirement source remains")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
