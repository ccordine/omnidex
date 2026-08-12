package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExistingRepositoryWorkflowHasNoOutputBlindCognitionSidecar(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{
			"runRepositoryCognitionShadow", "repository_cognition_shadow",
			"cognitionruntime", "cognitionpolicy", "cognitionstore",
			"CognitionDecision", "CognitionBrain",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("worker production source %s retains rejected cognition authority %q", file, forbidden)
			}
		}
	}
}

func TestWorkerOptionsExposeNoUniversalCognitionBrain(t *testing.T) {
	t.Parallel()
	for _, typ := range []reflect.Type{reflect.TypeOf(Options{}), reflect.TypeOf(Service{})} {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if strings.Contains(strings.ToLower(field.Name), "cognitionbrain") {
				t.Fatalf("%s retains universal cognition brain field %s", typ.Name(), field.Name)
			}
		}
	}
}

func TestExistingRepositoryWorkflowConsumesItsChangeContract(t *testing.T) {
	t.Parallel()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "v3_existing_repository_workflow.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundBuild := false
	foundApply := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "buildExistingRepositoryChangeContract":
			foundBuild = true
		case "applyExistingRepositoryChangeContract":
			foundApply = true
		}
		return true
	})
	if !foundBuild || !foundApply {
		t.Fatalf("existing repository workflow build=%t apply=%t; contract is not consumed", foundBuild, foundApply)
	}
}
