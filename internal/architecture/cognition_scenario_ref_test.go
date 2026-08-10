package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestProductionCognitionScenarioRefExposesOnlyIDAndSHA256(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "cognition"))
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("inspect cognition package: %v", err)
	}
	found, violations, err := scenarioRefContract(root)
	if err != nil {
		t.Fatalf("scan ScenarioRef contract: %v", err)
	}
	if found != 1 {
		t.Errorf("production cognition requires exactly one ScenarioRef declaration; found %d", found)
	}
	for _, violation := range violations {
		t.Errorf("production cognition ScenarioRef exposes non-public authority: %s", violation)
	}
}

func TestScenarioRefContractScannerRejectsAdditionalAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeArchitectureFixture(t, filepath.Join(root, "scenario.go"), `package cognition
type ScenarioID string
type ScenarioRef struct {
	ID ScenarioID
	SHA256 string
	Version string
	seed int64
}
`)
	found, violations, err := scenarioRefContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 || len(violations) != 2 {
		t.Fatalf("ScenarioRef found=%d violations=%v", found, violations)
	}
}

func scenarioRefContract(root string) (int, []string, error) {
	found := 0
	violations := []string{}
	err := walkProductionGo(root, func(path string, raw []byte) error {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, raw, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, declaration := range parsed.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "ScenarioRef" {
					continue
				}
				found++
				violations = append(violations, validateScenarioRef(path, typeSpec)...)
			}
		}
		return nil
	})
	return found, violations, err
}

func validateScenarioRef(path string, specification *ast.TypeSpec) []string {
	if specification.Assign.IsValid() || specification.TypeParams != nil {
		return []string{path + " declares ScenarioRef as an alias or generic type"}
	}
	structure, ok := specification.Type.(*ast.StructType)
	if !ok {
		return []string{path + " declares ScenarioRef as a non-struct type"}
	}
	allowed := map[string]string{"ID": "ScenarioID", "SHA256": "string"}
	present := map[string]bool{"ID": false, "SHA256": false}
	violations := []string{}
	for _, field := range structure.Fields.List {
		kind, namedField := field.Type.(*ast.Ident)
		if len(field.Names) == 0 {
			violations = append(violations, path+" embeds authority in ScenarioRef")
			continue
		}
		for _, name := range field.Names {
			expectedType, exists := allowed[name.Name]
			if !exists {
				violations = append(violations, path+" exposes ScenarioRef."+name.Name)
				continue
			}
			if !namedField || kind.Name != expectedType {
				violations = append(violations, path+" requires ScenarioRef."+name.Name+" to use "+expectedType)
				continue
			}
			present[name.Name] = true
		}
	}
	for name, exists := range present {
		if !exists {
			violations = append(violations, path+" omits ScenarioRef."+name)
		}
	}
	return violations
}
