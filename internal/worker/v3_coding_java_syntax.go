package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func javaNamedSyntaxChildren(node *treesitter.Node) []*treesitter.Node {
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
