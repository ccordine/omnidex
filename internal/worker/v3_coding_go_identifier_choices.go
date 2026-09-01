package worker

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

func directCodingGoIdentifierChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failed string,
	failedStart int,
) ([]assemblyline.OpaqueModelChoice, error) {
	compiled, err := gofragment.CompileNewFunctionSignature(input.Signature)
	if err != nil {
		return nil, err
	}
	prefix := "package fragment\n\n" + compiled.Canonical + " {\n"
	file, err := parser.ParseFile(token.NewFileSet(), "", prefix+body+"\n}", 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go scope for identifier choices: %w", err)
	}
	var function *ast.FuncDecl
	for _, declaration := range file.Decls {
		if candidate, ok := declaration.(*ast.FuncDecl); ok {
			function = candidate
			break
		}
	}
	if function == nil {
		return nil, fmt.Errorf("Go scope has no code-owned function declaration")
	}
	failedPosition := token.Pos(len(prefix) + failedStart + 1)
	candidates := make([]directCodingIdentifierCandidate, 0)
	addFields := func(fields *ast.FieldList, role string) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: name.Name, role: role,
				})
			}
		}
	}
	addFields(function.Type.Params, "function parameter")
	for _, name := range directCodingGoPermittedValueNames(input) {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted direct value",
		})
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE && value.End() < failedPosition &&
				goNodeScopeContains(function.Body, value, failedPosition) {
				for _, expression := range value.Lhs {
					if name, ok := expression.(*ast.Ident); ok {
						candidates = append(candidates, directCodingIdentifierCandidate{
							name: name.Name, role: "local value",
						})
					}
				}
			}
		case *ast.RangeStmt:
			if value.Body != nil && value.Body.Pos() <= failedPosition &&
				failedPosition <= value.Body.End() {
				for _, expression := range []ast.Expr{value.Key, value.Value} {
					if name, ok := expression.(*ast.Ident); ok {
						candidates = append(candidates, directCodingIdentifierCandidate{
							name: name.Name, role: "local iteration value",
						})
					}
				}
			}
		case *ast.ValueSpec:
			if value.End() < failedPosition &&
				goNodeScopeContains(function.Body, value, failedPosition) {
				for _, name := range value.Names {
					candidates = append(candidates, directCodingIdentifierCandidate{
						name: name.Name, role: "local value",
					})
				}
			}
		case *ast.FuncLit:
			if value.Body != nil && value.Body.Pos() <= failedPosition &&
				failedPosition <= value.Body.End() {
				addFields(value.Type.Params, "function parameter")
			}
		}
		return true
	})
	candidates = directCodingTrialIdentifierCandidates(
		body, failedStart, failedStart+len(failed), candidates,
		func(trial string) error {
			_, err := validateDirectCodingGoFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices("Go", failed, candidates)
}

func directCodingGoPermittedValueNames(input assemblyline.FragmentGenerationInput) []string {
	set := make(map[string]struct{})
	for _, authority := range append(
		append([]string(nil), input.Capabilities...), input.PermittedSymbols...,
	) {
		text := strings.TrimSpace(authority)
		if expression, err := parser.ParseExpr(text); err == nil {
			if identifier, ok := expression.(*ast.Ident); ok {
				set[identifier.Name] = struct{}{}
				continue
			}
		}
		source := "package fragment\n" + text
		file, err := parser.ParseFile(token.NewFileSet(), "", source, 0)
		if err != nil && strings.HasPrefix(text, "func ") {
			file, err = parser.ParseFile(token.NewFileSet(), "", source+" {}", 0)
		}
		if err != nil {
			continue
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				set[value.Name.Name] = struct{}{}
			case *ast.GenDecl:
				if value.Tok != token.VAR && value.Tok != token.CONST {
					continue
				}
				for _, spec := range value.Specs {
					if values, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range values.Names {
							set[name.Name] = struct{}{}
						}
					}
				}
			}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func goNodeScopeContains(root *ast.BlockStmt, declaration ast.Node, at token.Pos) bool {
	if root == nil || declaration == nil {
		return false
	}
	containing := root
	ast.Inspect(root, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok || block.Pos() > declaration.Pos() || declaration.End() > block.End() {
			return true
		}
		if block.Pos() >= containing.Pos() && block.End() <= containing.End() {
			containing = block
		}
		return true
	})
	return containing.Pos() <= at && at <= containing.End()
}
