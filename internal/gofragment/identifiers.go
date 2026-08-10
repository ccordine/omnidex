package gofragment

import (
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
)

func validateIdentifiers(current, candidate *ast.FuncDecl, permitted []string) error {
	allowed := identifierSet(current)
	for _, value := range permitted {
		for _, identifier := range scanIdentifiers(value) {
			allowed[identifier] = struct{}{}
		}
	}
	for identifier := range declaredIdentifiers(candidate) {
		allowed[identifier] = struct{}{}
	}
	for _, identifier := range scanIdentifiers(predeclaredIdentifiers) {
		allowed[identifier] = struct{}{}
	}
	var rejected string
	ast.Inspect(candidate, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return true
		}
		if _, exists := allowed[identifier.Name]; !exists && rejected == "" {
			rejected = identifier.Name
		}
		return true
	})
	if rejected != "" {
		return fmt.Errorf("Go fragment references undeclared capability %q", rejected)
	}
	return nil
}

func identifierSet(node ast.Node) map[string]struct{} {
	out := make(map[string]struct{})
	ast.Inspect(node, func(current ast.Node) bool {
		if identifier, ok := current.(*ast.Ident); ok {
			out[identifier.Name] = struct{}{}
		}
		return true
	})
	return out
}

func declaredIdentifiers(function *ast.FuncDecl) map[string]struct{} {
	out := make(map[string]struct{})
	ast.Inspect(function, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE {
				for _, expression := range value.Lhs {
					if identifier, ok := expression.(*ast.Ident); ok {
						out[identifier.Name] = struct{}{}
					}
				}
			}
		case *ast.RangeStmt:
			for _, expression := range []ast.Expr{value.Key, value.Value} {
				if identifier, ok := expression.(*ast.Ident); ok {
					out[identifier.Name] = struct{}{}
				}
			}
		case *ast.ValueSpec:
			for _, identifier := range value.Names {
				out[identifier.Name] = struct{}{}
			}
		}
		return true
	})
	return out
}

func scanIdentifiers(value string) []string {
	identifiers := make([]string, 0)
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(value))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(value), nil, 0)
	for {
		_, current, literal := lexer.Scan()
		if current == token.EOF {
			break
		}
		if current == token.IDENT {
			identifiers = append(identifiers, literal)
		}
	}
	return identifiers
}

const predeclaredIdentifiers = "true false nil iota int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr float32 float64 complex64 complex128 bool byte rune string error any comparable append cap clear close complex copy delete imag len make max min new panic print println real recover"
