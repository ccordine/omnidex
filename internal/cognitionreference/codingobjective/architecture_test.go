package codingobjective

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodingObjectivePackageDeclaresNoModelToolOrActionSurface(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"internal/llm", "internal/ollama", "cognitionpolicy", "cognitionruntime",
			"RenderPortableJob", "BuildGoFragmentModificationPrompt",
			"Return exactly one raw", "tool_calls", "evidence_refs", "expected_effect",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden provider/agent/prompt surface %q", path, forbidden)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok {
				return true
			}
			for _, name := range field.Names {
				switch name.Name {
				case "Tools", "Actions", "Action", "Arguments", "EvidenceRefs", "ExpectedEffect", "Proposals", "Attention":
					t.Errorf("%s exposes forbidden model authority field %q", path, name.Name)
				}
			}
			return true
		})
	}
}
