package labyrinth

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const cognitionImport = "github.com/gryph/omnidex/internal/cognition"

func TestSymbolicKernelDependsOnlyOnCognitionAndStandardLibrary(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("decode import in %s: %v", path, err)
			}
			if name == cognitionImport || !strings.Contains(strings.Split(name, "/")[0], ".") {
				continue
			}
			t.Errorf("%s imports %q; the symbolic kernel may depend only on cognition and the standard library", path, name)
		}
	}
}
