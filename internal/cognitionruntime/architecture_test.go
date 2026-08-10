package cognitionruntime

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestProductionRuntimeImportsOnlyTheDomainContract(t *testing.T) {
	t.Parallel()
	directory := runtimeDirectory(t)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(
			token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, parser.ImportsOnly,
		)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if strings.HasPrefix(path, "github.com/gryph/omnidex/internal/") &&
				path != "github.com/gryph/omnidex/internal/cognition" {
				t.Fatalf("%s imports forbidden subsystem %q", entry.Name(), path)
			}
		}
	}
}

func TestProductionRuntimeHasNoTranscriptOracleOrGauntletPath(t *testing.T) {
	t.Parallel()
	directory := runtimeDirectory(t)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ToLower(string(raw))
		for _, forbidden := range []string{
			"transcript", "oracle", "benchmark", "cognitiongauntlet", "labyrinth",
			"hidden state", "hidden_state", "fallback", "raw shell",
			"internal/queue", "internal/worker",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains forbidden production path %q", entry.Name(), forbidden)
			}
		}
	}
}

func TestCompletionEvaluatorCannotReceiveModelDecisionOrMutationAuthority(t *testing.T) {
	t.Parallel()
	evaluator := reflect.TypeOf((*CompletionEvaluator)(nil)).Elem()
	method, exists := evaluator.MethodByName("Evaluate")
	if !exists || method.Type.NumIn() != 2 || method.Type.In(1) != reflect.TypeOf(CompletionRequest{}) {
		t.Fatalf("CompletionEvaluator.Evaluate = %v", method.Type)
	}
	request := reflect.TypeOf(CompletionRequest{})
	decision := reflect.TypeOf(cognition.CognitionDecision{})
	environment := reflect.TypeOf((*cognition.Environment)(nil)).Elem()
	for index := 0; index < request.NumField(); index++ {
		if request.Field(index).Type == decision || request.Field(index).Type.Implements(environment) {
			t.Fatalf("completion request receives forbidden field %q", request.Field(index).Name)
		}
	}
}

func runtimeDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition runtime directory")
	}
	return filepath.Dir(file)
}
