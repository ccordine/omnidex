package architecture

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

func TestModelPromptInstructionsDoNotDescribeFrameworkControl(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		" agent ",
		" agents ",
		" worker ",
		" workers ",
		" orchestrator ",
		" tool call ",
		" tool calls ",
		" tool choice ",
		" tool selection ",
		" control plane ",
		" workflow decision ",
		" workflow control ",
		" permission to continue ",
		" approval to continue ",
		" completion status ",
		" completion claim ",
		" completeness review ",
		" quality review ",
		" retry decision ",
		" task queue ",
		" downstream code ",
		" code owns ",
		" code alone ",
		" code-owned ",
		" code-proven ",
		" code-bound ",
		" code-established ",
		" code-selected ",
		" code-reserved ",
		" source-owning ",
		" authorization authority ",
		" authorize the candidate ",
		"code_owned_",
		"code_proven_",
		"code_bound_",
		"code_established_",
		"code_selected_",
		"code_reserved_",
		"exact_output_limit_evidence",
		" later station ",
		" separate station ",
		" this call sees ",
		" independently sieve ",
		" authorize or discard ",
		" authorizes and resolves ",
		" not canon generation ",
		" decide completeness ",
		" review accepted ",
		" review retained ",
		" reopen accepted ",
		" revoke accepted ",
	}

	for _, scan := range []struct {
		root      string
		nameParts []string
	}{
		{root: filepath.Clean(filepath.Join("..", "assemblyline")), nameParts: []string{"prompt"}},
		{root: filepath.Clean(filepath.Join("..", "worker")), nameParts: []string{"prompt", "contract", "instruction", "behavior"}},
	} {
		entries, err := os.ReadDir(scan.root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(scan.root, name)
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil || !functionNameContains(
					function.Name.Name, scan.nameParts,
				) {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					if call, ok := node.(*ast.CallExpr); ok && isPromptDiagnosticCall(call.Fun) {
						return false
					}
					literal, ok := node.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						return true
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("unquote %s %s: %v", path, function.Name.Name, err)
					}
					framed := " " + strings.ToLower(value) + " "
					for _, term := range forbidden {
						if strings.Contains(framed, term) {
							t.Errorf(
								"model prompt builder %s in %s contains framework-control language %q",
								function.Name.Name, path, strings.TrimSpace(term),
							)
						}
					}
					return true
				})
			}
		}
	}
}

func functionNameContains(name string, parts []string) bool {
	lower := strings.ToLower(name)
	for _, part := range parts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func isPromptDiagnosticCall(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return (identifier.Name == "fmt" && selector.Sel.Name == "Errorf") ||
		(identifier.Name == "errors" && selector.Sel.Name == "New")
}
