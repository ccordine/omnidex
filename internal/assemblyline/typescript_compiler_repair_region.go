package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func NewTypeScriptCompilerRepairRegion(
	current string,
	tsx bool,
	line int,
	column int,
	bindings []TypeScriptRepairBinding,
	unavailableBindings ...[]TypeScriptRepairBinding,
) (TypeScriptFragmentRepairRegion, error) {
	return NewTypeScriptCompilerRepairRegionWithEvidence(
		current, tsx, line, column, bindings, nil, unavailableBindings...,
	)
}

func NewTypeScriptCompilerRepairRegionWithEvidence(
	current string,
	tsx bool,
	line int,
	column int,
	bindings []TypeScriptRepairBinding,
	expressionEvidence []TypeScriptRepairExpressionEvidence,
	unavailableBindings ...[]TypeScriptRepairBinding,
) (TypeScriptFragmentRepairRegion, error) {
	source := strings.TrimSpace(strings.ReplaceAll(current, "\r\n", "\n"))
	if source == "" || !utf8.ValidString(source) || strings.Contains(source, "\r") {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript compiler repair requires one normalized current declaration",
		)
	}
	lines := strings.Split(source, "\n")
	if line < 1 || line > len(lines) || column < 1 {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript compiler repair location line %d column %d is outside the current declaration",
			line, column,
		)
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	language := typescript.LanguageTypescript()
	if tsx {
		language = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf("configure TypeScript compiler repair parser: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf("TypeScript compiler repair parser returned no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf("TypeScript compiler repair requires parser-valid current source")
	}
	point := treesitter.Point{Row: uint(line - 1), Column: uint(column - 1)}
	owner := typeScriptCompilerRepairOwner(root, point)
	if owner == nil {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript compiler repair location line %d column %d has no exact AST owner",
			line, column,
		)
	}
	start := int(owner.StartPosition().Row) + 1
	end := int(owner.EndPosition().Row) + 1
	if end > len(lines) {
		end = len(lines)
	}
	region := TypeScriptFragmentRepairRegion{
		Kind:      TypeScriptRepairRegionCompilerOwner,
		StartLine: start, EndLine: end,
		Source:             strings.Join(lines[start-1:end], "\n"),
		Bindings:           append([]TypeScriptRepairBinding(nil), bindings...),
		ExpressionEvidence: append([]TypeScriptRepairExpressionEvidence(nil), expressionEvidence...),
	}
	if len(unavailableBindings) > 1 {
		return TypeScriptFragmentRepairRegion{}, fmt.Errorf(
			"TypeScript compiler repair region accepts at most one unavailable-binding inventory",
		)
	}
	if len(unavailableBindings) == 1 {
		region.UnavailableBindings = append(
			[]TypeScriptRepairBinding(nil), unavailableBindings[0]...,
		)
	}
	if err := region.validate(); err != nil {
		return TypeScriptFragmentRepairRegion{}, err
	}
	return region, nil
}

func typeScriptCompilerRepairOwner(root *treesitter.Node, point treesitter.Point) *treesitter.Node {
	if root == nil {
		return nil
	}
	node := root.NamedDescendantForPointRange(point, point)
	if node == nil {
		return nil
	}
	var functionBody *treesitter.Node
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent() {
		if ancestor.Kind() == "function_declaration" {
			functionBody = ancestor.ChildByFieldName("body")
			break
		}
	}
	if functionBody == nil {
		return nil
	}
	for ancestor := node; ancestor != nil && ancestor.Id() != functionBody.Id(); ancestor = ancestor.Parent() {
		switch ancestor.Kind() {
		case "jsx_expression", "jsx_attribute", "jsx_element", "jsx_self_closing_element":
			return ancestor
		}
		if parent := ancestor.Parent(); parent != nil && parent.Id() == functionBody.Id() {
			return ancestor
		}
	}
	return functionBody.Parent()
}
