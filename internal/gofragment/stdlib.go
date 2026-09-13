package gofragment

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"sync"
)

// This technical index contains only compiler-owned public package/symbol names.
// It is never model context. The selected toolchain checks signatures and types
// when the complete dependency graph is compiled.
//
//go:embed stdlib_go124_linux_amd64.txt
var standardLibraryIndex string

type standardLibraryPackage struct {
	path    string
	exports map[string]bool
}

var standardLibraryPackages = sync.OnceValues(func() (map[string][]standardLibraryPackage, error) {
	packages := map[string][]standardLibraryPackage{}
	for _, line := range strings.Split(strings.TrimSpace(standardLibraryIndex), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !token.IsIdentifier(fields[0]) {
			return nil, fmt.Errorf("invalid Go standard-library index record")
		}
		pkg := standardLibraryPackage{path: fields[1], exports: map[string]bool{}}
		for _, name := range fields[2:] {
			if !token.IsIdentifier(name) || !ast.IsExported(name) {
				return nil, fmt.Errorf("invalid Go standard-library export %q", name)
			}
			pkg.exports[name] = true
		}
		packages[fields[0]] = append(packages[fields[0]], pkg)
	}
	return packages, nil
})

// StandardLibraryImports derives exact imports from qualified source references.
// Locals and explicitly supplied project capabilities retain their own authority.
func StandardLibraryImports(source string, permitted []string) ([]string, error) {
	function, err := parseOneFunction(source, false)
	if err != nil {
		return nil, err
	}
	allowed, err := identifierAuthority(function, permitted)
	if err != nil {
		return nil, err
	}
	paths, _, err := standardLibraryReferences(function, allowed)
	return paths, err
}

func standardLibraryReferences(function *ast.FuncDecl, allowed map[string]struct{}) ([]string, map[*ast.Ident]bool, error) {
	packages, err := standardLibraryPackages()
	if err != nil {
		return nil, nil, err
	}
	imports := map[string]string{}
	nodes := map[*ast.Ident]bool{}
	var resolutionErr error
	ast.Inspect(function.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || resolutionErr != nil {
			return resolutionErr == nil
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || qualifier.Obj != nil {
			return true
		}
		if _, exists := allowed[qualifier.Name]; exists {
			return true
		}
		candidates := packages[qualifier.Name]
		if len(candidates) == 0 {
			return true
		}
		var matching []string
		for _, candidate := range candidates {
			if candidate.exports[selector.Sel.Name] {
				matching = append(matching, candidate.path)
			}
		}
		if len(matching) != 1 {
			resolutionErr = fmt.Errorf("Go standard-library reference %s.%s resolves to %d packages; one exact package is required", qualifier.Name, selector.Sel.Name, len(matching))
			return false
		}
		if previous, exists := imports[qualifier.Name]; exists && previous != matching[0] {
			resolutionErr = fmt.Errorf("Go qualifier %s refers to conflicting standard-library packages", qualifier.Name)
			return false
		}
		imports[qualifier.Name] = matching[0]
		nodes[qualifier], nodes[selector.Sel] = true, true
		return true
	})
	if resolutionErr != nil {
		return nil, nil, resolutionErr
	}
	paths := make([]string, 0, len(imports))
	for _, imported := range imports {
		paths = append(paths, imported)
	}
	sort.Strings(paths)
	return paths, nodes, nil
}
