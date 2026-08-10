package cognition

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPolicyBoundaryCannotReceiveEnvironment(t *testing.T) {
	t.Parallel()
	policyType := reflect.TypeOf((*Policy)(nil)).Elem()
	method, exists := policyType.MethodByName("Decide")
	if !exists {
		t.Fatal("Policy.Decide is missing")
	}
	for index := 0; index < method.Type.NumIn(); index++ {
		if method.Type.In(index) == reflect.TypeOf((*Environment)(nil)).Elem() {
			t.Fatal("Policy.Decide received Environment mutation authority")
		}
	}
	if method.Type.NumIn() != 2 || method.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() ||
		method.Type.In(1) != reflect.TypeOf(RuntimeSnapshot{}) {
		t.Fatalf("Policy.Decide inputs = %v, want context.Context and RuntimeSnapshot", method.Type)
	}
}

func TestRuntimeSnapshotHasNoExternallyMutableFields(t *testing.T) {
	t.Parallel()
	typeOfSnapshot := reflect.TypeOf(RuntimeSnapshot{})
	for index := 0; index < typeOfSnapshot.NumField(); index++ {
		field := typeOfSnapshot.Field(index)
		if field.PkgPath == "" {
			t.Fatalf("RuntimeSnapshot field %q is exported", field.Name)
		}
	}
}

func TestCoordinatorProductionFilesHaveNoEnvironmentMutationOrForbiddenImports(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition package directory")
	}
	directory := filepath.Dir(currentFile)
	for _, name := range []string{"coordinator.go", "runtime_snapshot.go"} {
		path := filepath.Join(directory, name)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range parsed.Decls {
			imports, ok := declaration.(*ast.GenDecl)
			if !ok || imports.Tok != token.IMPORT {
				continue
			}
			for _, spec := range imports.Specs {
				path := strings.Trim(spec.(*ast.ImportSpec).Path.Value, `"`)
				if strings.HasPrefix(path, "github.com/gryph/omnidex/internal/") {
					t.Fatalf("%s imports production subsystem %q", name, path)
				}
			}
		}
		source, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse source %s: %v", name, err)
		}
		ast.Inspect(source, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Apply" {
				t.Errorf("%s invokes an Apply mutation method", name)
			}
			return true
		})
	}
}

func TestCognitionProductionHasNoSubsystemImports(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cognition package directory")
	}
	directory := filepath.Dir(currentFile)
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
				path != "github.com/gryph/omnidex/internal/exactjson" {
				t.Fatalf("%s imports subsystem %q", entry.Name(), path)
			}
		}
	}
}
