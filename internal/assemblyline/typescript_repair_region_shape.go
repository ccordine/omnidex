package assemblyline

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func validateTypeScriptCompilerRepairReplacementShape(
	region TypeScriptFragmentRepairRegion,
	replacement string,
) error {
	if region.Kind != TypeScriptRepairRegionCompilerOwner {
		return nil
	}
	_, constrainedTSX, err := typeScriptCompilerRepairExpressionOwnerKind(region.Source, true)
	if err != nil {
		return err
	}
	_, constrainedTS, err := typeScriptCompilerRepairExpressionOwnerKind(region.Source, false)
	if err != nil {
		return err
	}
	if !constrainedTSX && !constrainedTS {
		return nil
	}
	gotTSX, preservedTSX, err := typeScriptCompilerRepairExpressionOwnerKind(replacement, true)
	if err != nil {
		return err
	}
	gotTS, preservedTS, err := typeScriptCompilerRepairExpressionOwnerKind(replacement, false)
	if err != nil {
		return err
	}
	if !preservedTSX && !preservedTS {
		return fmt.Errorf("TypeScript compiler repair replacement changed its single expression owner")
	}
	if (preservedTSX && gotTSX == "string") || (preservedTS && gotTS == "string") {
		return fmt.Errorf("TypeScript compiler repair replacement cannot replace an operation with a string literal statement")
	}
	return nil
}

func typeScriptCompilerRepairExpressionOwnerKind(
	source string,
	tsx bool,
) (string, bool, error) {
	const prefix = "function omnidexCompilerRepairOwner() {\n"
	wrapped := prefix + source + "\n}"
	parser := treesitter.NewParser()
	language := typescript.LanguageTypescript()
	if tsx {
		language = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		parser.Close()
		return "", false, fmt.Errorf("configure TypeScript compiler repair shape parser: %w", err)
	}
	defer parser.Close()
	tree := parser.Parse([]byte(wrapped), nil)
	if tree == nil {
		return "", false, fmt.Errorf("TypeScript compiler repair shape parser returned no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.NamedChildCount() != 1 {
		return "", false, nil
	}
	declaration := root.NamedChild(0)
	if declaration == nil || declaration.Kind() != "function_declaration" {
		return "", false, nil
	}
	body := declaration.ChildByFieldName("body")
	if body == nil || body.HasError() {
		return "", false, nil
	}
	statements := make([]*treesitter.Node, 0, body.NamedChildCount())
	for index := uint(0); index < body.NamedChildCount(); index++ {
		child := body.NamedChild(index)
		if child != nil && child.Kind() != "comment" {
			statements = append(statements, child)
		}
	}
	if len(statements) != 1 || statements[0].Kind() != "expression_statement" ||
		statements[0].NamedChildCount() != 1 {
		return "", false, nil
	}
	expression := statements[0].NamedChild(0)
	if expression == nil {
		return "", false, nil
	}
	return expression.Kind(), true, nil
}
