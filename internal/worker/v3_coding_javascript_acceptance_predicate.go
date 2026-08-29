package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func javaScriptAcceptanceAssertion(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) (string, bool) {
	if node == nil || node.Kind() != "call_expression" {
		return "", false
	}
	callee := node.ChildByFieldName("function")
	arguments := node.ChildByFieldName("arguments")
	if callee == nil || arguments == nil {
		return "", false
	}
	name := javaScriptNodeText(source, callee)
	values := javaScriptAcceptanceArguments(arguments)
	switch name {
	case "assert", "assert.ok":
		if len(values) < 1 || len(values) > 2 {
			return "", false
		}
		observation, expected, operator, valid := javaScriptAcceptanceComparison(
			source, values[0], resultNames,
		)
		if !valid || (len(values) == 2 && !javaScriptAcceptanceExpected(source, values[1])) {
			return "", false
		}
		return name + ":" + observation + operator + expected, true
	case "assert.equal", "assert.strictEqual", "assert.deepEqual",
		"assert.notEqual", "assert.notStrictEqual", "assert.notDeepEqual":
		if len(values) < 2 || len(values) > 3 {
			return "", false
		}
		observation, valid := javaScriptAcceptanceObservation(source, values[0], resultNames)
		if !valid || !javaScriptAcceptanceExpected(source, values[1]) ||
			(len(values) == 3 && !javaScriptAcceptanceExpected(source, values[2])) {
			return "", false
		}
		return name + ":" + observation + "," + javaScriptNodeText(source, values[1]), true
	default:
		return "", false
	}
}

func javaScriptAcceptanceComparison(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) (string, string, string, bool) {
	node = javaScriptUnwrapParentheses(node)
	if node == nil || node.Kind() != "binary_expression" {
		return "", "", "", false
	}
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return "", "", "", false
	}
	operator := strings.TrimSpace(string(source[left.EndByte():right.StartByte()]))
	switch operator {
	case "===", "!==", "==", "!=", "<", "<=", ">", ">=":
	default:
		return "", "", "", false
	}
	if observation, valid := javaScriptAcceptanceObservation(source, left, resultNames); valid &&
		javaScriptAcceptanceExpected(source, right) {
		return observation, javaScriptNodeText(source, right), operator, true
	}
	if observation, valid := javaScriptAcceptanceObservation(source, right, resultNames); valid &&
		javaScriptAcceptanceExpected(source, left) {
		return observation, javaScriptNodeText(source, left), operator, true
	}
	return "", "", "", false
}

func javaScriptAcceptanceObservation(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) (string, bool) {
	node = javaScriptUnwrapParentheses(node)
	if node == nil || node.Kind() != "member_expression" {
		return "", false
	}
	object := node.ChildByFieldName("object")
	property := node.ChildByFieldName("property")
	if object == nil || property == nil || property.Kind() != "property_identifier" {
		return "", false
	}
	root := object
	for root != nil && root.Kind() == "member_expression" {
		childProperty := root.ChildByFieldName("property")
		if childProperty == nil || childProperty.Kind() != "property_identifier" {
			return "", false
		}
		root = root.ChildByFieldName("object")
	}
	if root == nil || root.Kind() != "identifier" {
		return "", false
	}
	if _, exists := resultNames[javaScriptNodeText(source, root)]; !exists {
		return "", false
	}
	return javaScriptNodeText(source, node), true
}

func javaScriptAcceptanceExpected(source []byte, node *treesitter.Node) bool {
	node = javaScriptUnwrapParentheses(node)
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "string", "template_string", "number", "true", "false", "null":
		return true
	case "array":
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if !javaScriptAcceptanceExpected(source, node.NamedChild(index)) {
				return false
			}
		}
		return true
	case "object":
		for index := uint(0); index < node.NamedChildCount(); index++ {
			pair := node.NamedChild(index)
			if pair.Kind() != "pair" || !javaScriptAcceptanceExpected(source, pair.ChildByFieldName("value")) {
				return false
			}
		}
		return true
	case "unary_expression":
		if node.NamedChildCount() != 1 {
			return false
		}
		value := javaScriptUnwrapParentheses(node.NamedChild(0))
		text := javaScriptNodeText(source, node)
		return value != nil && value.Kind() == "number" &&
			(strings.HasPrefix(text, "-") || strings.HasPrefix(text, "+"))
	default:
		return false
	}
}

func javaScriptAcceptanceArguments(arguments *treesitter.Node) []*treesitter.Node {
	values := make([]*treesitter.Node, 0, arguments.NamedChildCount())
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		values = append(values, arguments.NamedChild(index))
	}
	return values
}

func javaScriptUnwrapParentheses(node *treesitter.Node) *treesitter.Node {
	for node != nil && node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}
	return node
}

func javaScriptNodeText(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(string(source[node.StartByte():node.EndByte()]))
}
