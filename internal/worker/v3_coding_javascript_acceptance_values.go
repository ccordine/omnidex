package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func javaScriptAcceptanceLiteral(node *treesitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "string", "number", "true", "false", "null":
		return true
	case "identifier", "undefined":
		value := node.Utf8Text(source)
		return value == "undefined" || value == "NaN" || value == "Infinity"
	case "unary_expression":
		operator, argument := node.ChildByFieldName("operator"), node.ChildByFieldName("argument")
		return operator != nil && argument != nil && (operator.Kind() == "-" || operator.Kind() == "+") && argument.Kind() == "number"
	case "array":
		for _, item := range javaScriptAcceptanceChildren(node) {
			if !javaScriptAcceptanceLiteral(item, source) {
				return false
			}
		}
		return true
	case "object":
		for _, pair := range javaScriptAcceptanceChildren(node) {
			key := pair.ChildByFieldName("key")
			if pair.Kind() != "pair" || key == nil || (key.Kind() != "property_identifier" && key.Kind() != "string" && key.Kind() != "number") || !javaScriptAcceptanceLiteral(pair.ChildByFieldName("value"), source) {
				return false
			}
		}
		return true
	}
	return false
}

func javaScriptAcceptanceInput(node *treesitter.Node, source []byte) bool {
	if node.Kind() != "object" || !javaScriptAcceptanceLiteral(node, source) {
		return false
	}
	pairs := javaScriptAcceptanceChildren(node)
	if len(pairs) != 2 {
		return false
	}
	seen := make(map[string]bool, 2)
	for _, pair := range pairs {
		key := strings.Trim(pair.ChildByFieldName("key").Utf8Text(source), "'\"")
		value := pair.ChildByFieldName("value")
		if seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "arguments":
			if value.Kind() != "array" {
				return false
			}
			for _, argument := range javaScriptAcceptanceChildren(value) {
				if argument.Kind() != "string" {
					return false
				}
			}
		case "standardInput":
			if value.Kind() != "string" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
