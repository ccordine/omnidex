package cognitionstate

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionImportsOnlyPureStateBoundaries(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition-state package")
	}
	directory := filepath.Dir(currentFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"github.com/gryph/omnidex/internal/cognition":      true,
		"github.com/gryph/omnidex/internal/contextbuilder": true,
		"github.com/gryph/omnidex/internal/taskstate":      true,
		"github.com/gryph/omnidex/internal/workingset":     true,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, imported := range parsed.Imports {
			name := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(name, "github.com/gryph/omnidex/internal/") && !allowed[name] {
				t.Fatalf("%s imports forbidden subsystem %q", entry.Name(), name)
			}
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ToLower(string(raw))
		for _, forbidden := range []string{
			"transcript", "fallback", "raw shell", "cognitiongauntlet",
			"labyrinth", "roguelike", "oracle", "internal/queue", "internal/worker",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains forbidden state path %q", entry.Name(), forbidden)
			}
		}
	}
}
