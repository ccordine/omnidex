package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func javaExactMethodInvocation(
	source []byte,
	node *treesitter.Node,
	owner string,
	method string,
) bool {
	if node == nil || node.Kind() != "method_invocation" {
		return false
	}
	object, name := node.ChildByFieldName("object"), node.ChildByFieldName("name")
	return object != nil && object.Kind() == "identifier" && name != nil &&
		javaNodeText(object, source) == owner && javaNodeText(name, source) == method
}

func javaCallHasExactIdentifier(node *treesitter.Node, source []byte, name string) bool {
	arguments := javaCallArguments(node)
	return len(arguments) == 1 && arguments[0].Kind() == "identifier" &&
		javaNodeText(arguments[0], source) == name
}

func javaCallArguments(node *treesitter.Node) []*treesitter.Node {
	arguments := node.ChildByFieldName("arguments")
	if arguments == nil {
		return nil
	}
	values := make([]*treesitter.Node, 0, arguments.NamedChildCount())
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		if child := arguments.NamedChild(index); child != nil {
			values = append(values, child)
		}
	}
	return values
}

func javaCanonicalExpression(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		return javaCanonicalExpression(source, node.NamedChild(0))
	}
	if node.ChildCount() == 0 {
		return node.Kind() + ":" + strings.TrimSpace(javaNodeText(node, source))
	}
	var canonical strings.Builder
	canonical.WriteString(node.Kind())
	canonical.WriteByte('(')
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child == nil || child.Kind() == "line_comment" || child.Kind() == "block_comment" {
			continue
		}
		canonical.WriteString(javaCanonicalExpression(source, child))
		canonical.WriteByte(';')
	}
	canonical.WriteByte(')')
	return canonical.String()
}

func javaUnwrapParenthesizedExpression(node *treesitter.Node) *treesitter.Node {
	for node != nil && node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}
	return node
}

func javaWalkTree(node *treesitter.Node, visit func(*treesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for index := uint(0); index < node.ChildCount(); index++ {
		javaWalkTree(node.Child(index), visit)
	}
}
