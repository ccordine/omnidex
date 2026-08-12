package semanticreview

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSurfaceContainsNoAgentProviderOrModelRepairPath(t *testing.T) {
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
			"internal/llm", "internal/ollama", "qwenselector", "assemblyline",
			"SemanticFactProducer", "FragmentCorrection", "CognitionDecision",
			"tool_calls", "evidence_refs", "expected_effect", "attention_requests",
			"retry explanation", "simulation appraisal", "danger", "Failures are retried",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden authority or fixture surface %q", path, forbidden)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, raw, parser.AllErrors)
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
				case "Action", "Arguments", "Tools", "Tool", "Memory", "Ledger", "Completion", "ExecutorID", "OperationID", "ObjectiveKind":
					if findingStructField(parsed, field) {
						t.Errorf("%s exposes execution/completion field %q on a finding type", path, name.Name)
					}
				}
			}
			return true
		})
	}
}

func findingStructField(file *ast.File, wanted *ast.Field) bool {
	matched := false
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || (typeSpec.Name.Name != "ReviewFinding" && typeSpec.Name.Name != "FindingDefinition") {
			return true
		}
		structure, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range structure.Fields.List {
			if field == wanted {
				matched = true
			}
		}
		return false
	})
	return matched
}

func TestProductionFilesRemainSmall(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(raw), "\n") + 1; lines >= 300 {
			t.Errorf("%s has %d lines", entry.Name(), lines)
		}
	}
}
