package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserNumericIndexBindings map[uintptr]struct{}

func collectDirectCodingBrowserNumericIndexBindings(
	root *treesitter.Node,
) (directCodingBrowserNumericIndexBindings, error) {
	indices := make(directCodingBrowserNumericIndexBindings)
	var walk func(*treesitter.Node) error
	walk = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind() == "variable_declarator" {
			declaration := directCodingBrowserRuntimeDeclaration(node)
			name := node.ChildByFieldName("name")
			value := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("value"))
			if declaration != nil && declaration.Kind() == "lexical_declaration" &&
				name != nil && name.Kind() == "identifier" && value != nil &&
				value.Kind() == "number" && directCodingBrowserDeclarationIsConst(declaration) {
				indices[name.Id()] = struct{}{}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := walk(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return indices, nil
}

func directCodingBrowserDeclarationIsConst(node *treesitter.Node) bool {
	return node != nil && node.ChildCount() > 0 && node.Child(0).Kind() == "const"
}

func (indices directCodingBrowserNumericIndexBindings) permits(
	index *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	index = directCodingBrowserUnwrapRuntimeExpression(index)
	if index == nil {
		return false
	}
	if index.Kind() == "number" {
		return true
	}
	if index.Kind() != "identifier" {
		return false
	}
	name := directCodingBrowserRuntimeNodeText(source, index)
	var selected *directCodingBrowserRuntimeBinding
	selectedWidth := ^uint(0)
	ambiguous := false
	for bindingIndex := range bindings.byName[name] {
		binding := &bindings.byName[name][bindingIndex]
		if index.StartByte() < binding.start || index.EndByte() > binding.end {
			continue
		}
		width := binding.end - binding.start
		if width < selectedWidth {
			selected, selectedWidth, ambiguous = binding, width, false
		} else if width == selectedWidth {
			ambiguous = true
		}
	}
	if selected == nil || ambiguous {
		return false
	}
	_, permitted := indices[selected.declarationID]
	return permitted
}

func validateDirectCodingBrowserComputedProperty(
	node *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	indices directCodingBrowserNumericIndexBindings,
) error {
	if node == nil || node.Kind() != "subscript_expression" {
		return nil
	}
	index := node.ChildByFieldName("index")
	if _, resolved := javaScriptStaticPropertyName(source, index); resolved {
		return nil
	}
	if indices.permits(index, source, bindings) {
		return nil
	}
	return fmt.Errorf("browser public surface rejects unresolved computed property authority")
}
