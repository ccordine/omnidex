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

func TestOllamaEmbeddingHasOneAuthoritativeEndpointAndSchema(t *testing.T) {
	path := filepath.Join("..", "ollama", "client.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		`/api/embeddings`,
		"`json:\"prompt\"`",
		"`json:\"embedding\"`",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Ollama client retains legacy embedding contract %q", forbidden)
		}
	}
	for _, required := range []string{
		"`json:\"input\"`",
		"`json:\"embeddings\"`",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Ollama client omitted authoritative embedding contract %q", required)
		}
	}

	parsed, err := parser.ParseFile(token.NewFileSet(), path, raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	embedding := findArchitectureMethod(parsed, "Client", "Embedding")
	if embedding == nil {
		t.Fatal("(*ollama.Client).Embedding is missing")
	}

	providerRequests := 0
	directTransportRequests := 0
	endpoint := ""
	ast.Inspect(embedding.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "postJSON":
			providerRequests++
			if len(call.Args) >= 2 {
				if literal, ok := call.Args[1].(*ast.BasicLit); ok && literal.Kind == token.STRING {
					endpoint, _ = strconv.Unquote(literal.Value)
				}
			}
		case "Do":
			directTransportRequests++
		}
		return true
	})
	if providerRequests != 1 {
		t.Errorf("(*ollama.Client).Embedding makes %d postJSON calls, want exactly one", providerRequests)
	}
	if directTransportRequests != 0 {
		t.Errorf("(*ollama.Client).Embedding makes %d direct transport calls, want none", directTransportRequests)
	}
	if endpoint != "/api/embed" {
		t.Errorf("(*ollama.Client).Embedding endpoint=%q, want /api/embed", endpoint)
	}

	ast.Inspect(embedding.Body, func(node ast.Node) bool {
		switch loop := node.(type) {
		case *ast.ForStmt:
			if architectureNodeCallsMethod(loop.Body, "postJSON") {
				t.Error("(*ollama.Client).Embedding loops over its provider request")
			}
		case *ast.RangeStmt:
			if architectureNodeCallsMethod(loop.Body, "postJSON") {
				t.Error("(*ollama.Client).Embedding ranges over its provider request")
			}
		}
		return true
	})
}

func findArchitectureMethod(file *ast.File, receiverName, methodName string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != methodName || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		receiver := function.Recv.List[0].Type
		if pointer, ok := receiver.(*ast.StarExpr); ok {
			receiver = pointer.X
		}
		identifier, ok := receiver.(*ast.Ident)
		if ok && identifier.Name == receiverName {
			return function
		}
	}
	return nil
}

func architectureNodeCallsMethod(node ast.Node, method string) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		call, ok := current.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == method {
			found = true
			return false
		}
		return true
	})
	return found
}
