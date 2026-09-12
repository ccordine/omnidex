package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

// Rust's typechecker owns the input/result types. This check only establishes
// that setup and expected values do not depend on the observation under test.
func rustAcceptanceLiteral(node *treesitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "string_literal", "raw_string_literal", "char_literal", "integer_literal", "float_literal", "boolean_literal":
		return true
	case "identifier":
		return rustNodeText(source, node) == "None"
	case "reference_expression":
		return rustAcceptanceLiteral(node.ChildByFieldName("value"), source)
	case "unary_expression":
		children := rustAcceptanceChildren(node)
		return len(children) == 1 && node.Child(0).Kind() == "-" && (children[0].Kind() == "integer_literal" || children[0].Kind() == "float_literal")
	case "parenthesized_expression", "array_expression", "tuple_expression":
		return rustAcceptanceLiterals(rustAcceptanceChildren(node), source)
	case "struct_expression":
		name := rustNodeText(source, node.ChildByFieldName("name"))
		if name != "TaskInput" && name != "TaskResult" {
			return false
		}
		for _, field := range rustAcceptanceChildren(node.ChildByFieldName("body")) {
			switch field.Kind() {
			case "field_initializer":
				if !rustAcceptanceLiteral(field.ChildByFieldName("value"), source) {
					return false
				}
			case "base_field_initializer":
				if !rustAcceptanceLiterals(rustAcceptanceChildren(field), source) {
					return false
				}
			default:
				return false
			}
		}
		return true
	case "call_expression":
		return rustAcceptanceLiteralCall(node, source)
	case "macro_invocation":
		if rustNodeText(source, node.ChildByFieldName("macro")) != "vec" {
			return false
		}
		value, literalSource, closeTree, err := rustAcceptanceMacroExpression(node, source, "[", "]")
		if err != nil {
			return false
		}
		defer closeTree()
		return value.Kind() == "array_expression" && rustAcceptanceLiteral(value, literalSource)
	}
	return false
}

func rustAcceptanceLiterals(nodes []*treesitter.Node, source []byte) bool {
	for _, node := range nodes {
		if !rustAcceptanceLiteral(node, source) {
			return false
		}
	}
	return true
}

func rustAcceptanceLiteralCall(node *treesitter.Node, source []byte) bool {
	function := node.ChildByFieldName("function")
	if function == nil {
		return false
	}
	arguments := rustAcceptanceChildren(node.ChildByFieldName("arguments"))
	if !rustAcceptanceLiterals(arguments, source) {
		return false
	}
	if function.Kind() == "field_expression" {
		if len(arguments) != 0 || !rustAcceptanceLiteral(function.ChildByFieldName("value"), source) {
			return false
		}
		switch rustNodeText(source, function.ChildByFieldName("field")) {
		case "to_string", "to_owned", "into", "clone":
			return true
		}
		return false
	}
	switch rustNodeText(source, function) {
	case "String::new", "Vec::new", "CapabilityResults::new", "TaskInput::default", "TaskResult::default":
		return len(arguments) == 0
	case "String::from", "Vec::from", "CapabilityResults::from", "Some":
		return len(arguments) == 1
	}
	return false
}
