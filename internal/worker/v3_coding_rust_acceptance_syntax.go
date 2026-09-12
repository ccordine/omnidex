package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func rustAcceptanceChildren(node *treesitter.Node) []*treesitter.Node {
	var children []*treesitter.Node
	if node != nil {
		for index := uint(0); index < node.NamedChildCount(); index++ {
			child := node.NamedChild(index)
			if child.Kind() != "line_comment" && child.Kind() != "block_comment" {
				children = append(children, child)
			}
		}
	}
	return children
}

// Native macro arguments are token trees. Parse their exact contents as an
// expression to inspect the operands; no model interprets or rewrites them.
func rustAcceptanceMacroExpression(node *treesitter.Node, source []byte, prefix, suffix string) (*treesitter.Node, []byte, func(), error) {
	var tokens *treesitter.Node
	for _, child := range rustAcceptanceChildren(node) {
		if child.Kind() == "token_tree" {
			tokens = child
		}
	}
	if tokens == nil || tokens.EndByte()-tokens.StartByte() < 2 {
		return nil, nil, nil, fmt.Errorf("Rust acceptance macro has no bounded operands")
	}
	contents := string(source[tokens.StartByte()+1 : tokens.EndByte()-1])
	parsedSource := []byte("fn inspect() { let value = " + prefix + contents + suffix + "; }")
	root, closeTree, err := parseRustAuthorityTree(parsedSource)
	if err != nil {
		return nil, nil, nil, err
	}
	if root.NamedChildCount() == 1 && root.NamedChild(0).Kind() == "function_item" {
		statements := rustAcceptanceChildren(root.NamedChild(0).ChildByFieldName("body"))
		if len(statements) == 1 && statements[0].Kind() == "let_declaration" {
			if value := statements[0].ChildByFieldName("value"); value != nil {
				return value, parsedSource, closeTree, nil
			}
		}
	}
	closeTree()
	return nil, nil, nil, fmt.Errorf("Rust acceptance macro operands changed the parsed expression")
}
