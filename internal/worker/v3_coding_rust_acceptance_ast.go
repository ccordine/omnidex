package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func walkRustTree(node *treesitter.Node, visit func(*treesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for index := uint(0); index < node.ChildCount(); index++ {
		walkRustTree(node.Child(index), visit)
	}
}

func rustNodeText(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func rustUnwrapParenthesizedExpression(node *treesitter.Node) *treesitter.Node {
	for node != nil && node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}
	return node
}
