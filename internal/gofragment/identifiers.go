package gofragment

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strings"
)

// UndeclaredIdentifierViolation is declaration-relative parser evidence for
// the first unresolved identifier occurrence. Later occurrences remain
// untouched until ordinary validation reaches them after the exact splice.
type UndeclaredIdentifierViolation struct {
	Name      string
	StartByte int
	EndByte   int
}

func (violation *UndeclaredIdentifierViolation) Error() string {
	if violation == nil {
		return "Go undeclared identifier violation is nil"
	}
	return fmt.Sprintf("Go fragment references undeclared capability %q", violation.Name)
}

func validateIdentifiers(current, candidate *ast.FuncDecl, permitted []string) error {
	allowed := functionInterfaceIdentifierSet(current)
	for _, value := range permitted {
		projected, err := permittedIdentifierSet(value)
		if err != nil {
			return err
		}
		for identifier := range projected {
			allowed[identifier] = struct{}{}
		}
	}
	for _, identifier := range scanIdentifiers(predeclaredIdentifiers) {
		allowed[identifier] = struct{}{}
	}
	rejected := make(map[string]struct{})
	locations := make([]UndeclaredIdentifierViolation, 0, 1)
	ast.Inspect(candidate, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return true
		}
		_, externallyAllowed := allowed[identifier.Name]
		if !externallyAllowed && identifier.Obj == nil {
			rejected[identifier.Name] = struct{}{}
			locations = append(locations, UndeclaredIdentifierViolation{
				Name:      identifier.Name,
				StartByte: int(identifier.Pos()) - 1 - len(goFragmentFilePrefix),
				EndByte:   int(identifier.End()) - 1 - len(goFragmentFilePrefix),
			})
		}
		return true
	})
	if len(rejected) == 0 {
		return nil
	}
	if len(locations) > 0 && locations[0].StartByte >= 0 &&
		locations[0].EndByte > locations[0].StartByte {
		return &locations[0]
	}
	identifiers := make([]string, 0, len(rejected))
	for identifier := range rejected {
		identifiers = append(identifiers, identifier)
	}
	sort.Strings(identifiers)
	if len(identifiers) == 1 {
		return fmt.Errorf("Go fragment references undeclared capability %q", identifiers[0])
	}
	return fmt.Errorf("Go fragment references undeclared capabilities %q", identifiers)
}

func functionInterfaceIdentifierSet(function *ast.FuncDecl) map[string]struct{} {
	out := make(map[string]struct{})
	if function == nil {
		return out
	}
	out[function.Name.Name] = struct{}{}
	ast.Inspect(function.Type, func(current ast.Node) bool {
		if identifier, ok := current.(*ast.Ident); ok {
			out[identifier.Name] = struct{}{}
		}
		return true
	})
	return out
}

// permittedIdentifierSet projects only names a caller can actually reference
// from one code-owned capability declaration. Function parameter names are
// local to that declaration and therefore never become ambient authority in a
// generated function body.
func permittedIdentifierSet(value string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("Go permitted symbol declaration is empty")
	}
	if expression, err := parser.ParseExpr(value); err == nil {
		ast.Inspect(expression, func(node ast.Node) bool {
			if identifier, ok := node.(*ast.Ident); ok {
				out[identifier.Name] = struct{}{}
			}
			return true
		})
		return out, nil
	}
	source := goFragmentFilePrefix + value
	file, err := parser.ParseFile(token.NewFileSet(), "", source, parser.AllErrors)
	if err != nil && strings.HasPrefix(value, "func ") {
		file, err = parser.ParseFile(token.NewFileSet(), "", source+" {}", parser.AllErrors)
	}
	if err != nil {
		return nil, fmt.Errorf("parse Go permitted symbol declaration: %w", err)
	}
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			out[typed.Name.Name] = struct{}{}
			collectGoTypeReferences(out, typed.Type)
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch item := spec.(type) {
				case *ast.TypeSpec:
					out[item.Name.Name] = struct{}{}
					collectGoExposedTypeIdentifiers(out, item.Type)
				case *ast.ValueSpec:
					for _, name := range item.Names {
						out[name.Name] = struct{}{}
					}
					collectGoTypeReferences(out, item.Type)
				case *ast.ImportSpec:
					if item.Name != nil {
						out[item.Name.Name] = struct{}{}
					}
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Go permitted symbol declaration exposes no usable name")
	}
	return out, nil
}

func collectGoExposedTypeIdentifiers(out map[string]struct{}, node ast.Node) {
	switch typed := node.(type) {
	case *ast.StructType:
		for _, field := range typed.Fields.List {
			for _, name := range field.Names {
				out[name.Name] = struct{}{}
			}
		}
	case *ast.InterfaceType:
		for _, field := range typed.Methods.List {
			for _, name := range field.Names {
				out[name.Name] = struct{}{}
			}
		}
	}
	collectGoTypeReferences(out, node)
}

func collectGoTypeReferences(out map[string]struct{}, node ast.Node) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(current ast.Node) bool {
		field, ok := current.(*ast.Field)
		if ok {
			collectGoTypeReferences(out, field.Type)
			return false
		}
		if identifier, ok := current.(*ast.Ident); ok {
			out[identifier.Name] = struct{}{}
		}
		return true
	})
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
