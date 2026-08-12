package objectiveworkload_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReferenceHasNoAgentToolPersistenceOrProductionFallbackSurface(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		content := string(raw)
		for _, forbidden := range []string{
			"CognitionDecision", "ActionSchema", "EvidenceRefs", "ExpectedEffect",
			"AttentionRequest", "ToolCall", "tool_catalog", "database/sql", "pgx",
			"postgres", "redis", "cognitionruntime", "cognitionpolicy", "qwenselector",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("production source %s contains forbidden surface %q", entry.Name(), forbidden)
			}
		}
		if (entry.Name() == "run.go" || entry.Name() == "runtime_types.go" ||
			entry.Name() == "artifact.go" || entry.Name() == "graph.go") &&
			(strings.Contains(content, "PartitionStation") || strings.Contains(content, "PortableJob") ||
				strings.Contains(content, "assemblyline")) {
			t.Fatalf("deterministic runtime source %s imports semantic station authority", entry.Name())
		}
	}
}

func TestReferenceExportsNoMutationOrPlannerFunction(t *testing.T) {
	t.Parallel()
	root := packageRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"Compile": true, "Run": true}
	set := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !function.Name.IsExported() {
				continue
			}
			if !allowed[function.Name.Name] {
				t.Fatalf("unexpected exported function %s in %s", function.Name.Name, filepath.Base(path))
			}
		}
	}
}

func packageRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Dir(current)
}
