package repositoryobjective

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

func TestProductionPackageHasNoAgentToolOrMutationAuthority(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	allowedImports := map[string]bool{
		"context": true, "crypto/sha256": true, "encoding/hex": true,
		"errors": true, "fmt": true, "path/filepath": true, "reflect": true,
		"sort": true, "strings": true, "unicode/utf8": true,
		"github.com/gryph/omnidex/internal/cognitionreference":         true,
		"github.com/gryph/omnidex/internal/repository":                 true,
		"github.com/gryph/omnidex/internal/repository/adapters/golang": true,
		"github.com/gryph/omnidex/internal/repository/changeapply":     true,
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil || !allowedImports[name] {
				t.Errorf("%s imports forbidden authority %q", path, name)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			for _, forbidden := range []string{
				"CognitionDecision", "ActionSchema", "ToolCall", "ExecuteTool",
				"ApplyVerified", "StageChange", "WriteFile", "RemoveAll", "Plan",
				"StagedChange", "CandidateDeclaration", "changeapply.Input",
			} {
				if identifier.Name == forbidden {
					t.Errorf("%s contains forbidden model/mutation authority %q", path, forbidden)
				}
			}
			return true
		})
	}
}

func TestRepositoryObjectiveFilesRemainFocused(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(raw), "\n"); lines >= 300 {
			t.Errorf("%s has %d lines; split before 300", entry.Name(), lines)
		}
	}
}
