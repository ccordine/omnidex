package worker

import (
	"strconv"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func javaAcceptanceInput(node *treesitter.Node, source []byte) bool {
	arguments, ok := javaAcceptanceFactoryArguments(node, source, "Map", "of")
	if !ok || len(arguments) != 4 {
		return false
	}
	seen := make(map[string]bool, 2)
	for index := 0; index < len(arguments); index += 2 {
		key, err := strconv.Unquote(arguments[index].Utf8Text(source))
		if err != nil || seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "arguments":
			values, ok := javaAcceptanceFactoryArguments(arguments[index+1], source, "List", "of")
			if !ok {
				return false
			}
			for _, value := range values {
				if value.Kind() != "string_literal" {
					return false
				}
			}
		case "standardInput":
			if arguments[index+1].Kind() != "string_literal" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func javaAcceptanceLiteral(node *treesitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	if javaAcceptanceScalarLiteral(node, source) {
		return true
	}
	switch node.Kind() {
	case "string_literal":
		return true
	case "parenthesized_expression":
		children := javaNamedSyntaxChildren(node)
		return len(children) == 1 && javaAcceptanceLiteral(children[0], source)
	case "method_invocation":
		for _, factory := range [][2]string{{"Map", "of"}, {"List", "of"}, {"String", "valueOf"}, {"Integer", "valueOf"}, {"Long", "valueOf"}, {"Double", "valueOf"}, {"Boolean", "valueOf"}} {
			if arguments, ok := javaAcceptanceFactoryArguments(node, source, factory[0], factory[1]); ok {
				for _, argument := range arguments {
					if !javaAcceptanceLiteral(argument, source) {
						return false
					}
				}
				return true
			}
		}
	}
	return false
}

func javaAcceptanceScalarLiteral(node *treesitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal", "binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal", "character_literal", "true", "false", "null_literal":
		return true
	case "unary_expression":
		operator, operand := node.ChildByFieldName("operator"), node.ChildByFieldName("operand")
		return operator != nil && (operator.Utf8Text(source) == "-" || operator.Utf8Text(source) == "+") && javaAcceptanceScalarLiteral(operand, source)
	}
	return false
}

func javaAcceptanceFactoryArguments(node *treesitter.Node, source []byte, owner, method string) ([]*treesitter.Node, bool) {
	if node == nil || node.Kind() != "method_invocation" {
		return nil, false
	}
	object, name := node.ChildByFieldName("object"), node.ChildByFieldName("name")
	if object == nil || object.Kind() != "identifier" || object.Utf8Text(source) != owner || name == nil || name.Utf8Text(source) != method {
		return nil, false
	}
	return javaNamedSyntaxChildren(node.ChildByFieldName("arguments")), true
}
