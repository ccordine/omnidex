package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateDirectCodingRustAcceptance(ref assemblyline.SourceBlockRef, declaration string) error {
	if ref.Block.Role != assemblyline.SourceBlockTaskVerification || ref.Block.TaskID == "" {
		return fmt.Errorf("Rust acceptance requires one owned verification declaration")
	}
	source := []byte(declaration)
	root, closeTree, err := parseRustAuthorityTree(source)
	if err != nil {
		return err
	}
	defer closeTree()
	if root.NamedChildCount() != 1 || root.NamedChild(0).Kind() != "function_item" {
		return fmt.Errorf("Rust acceptance requires its exact function declaration")
	}
	body := root.NamedChild(0).ChildByFieldName("body")
	if body == nil || strings.TrimSpace(declaration[:body.StartByte()]) != ref.Block.Signature {
		return fmt.Errorf("Rust acceptance changed its code-owned declaration")
	}
	statements := rustAcceptanceChildren(body)
	if len(statements) < 2 {
		return fmt.Errorf("Rust acceptance must run the behavior and assert its observed result")
	}
	name, err := rustAcceptanceResultBinding(statements[0], source)
	if err != nil {
		return err
	}
	for _, statement := range statements[1:] {
		if !rustAcceptanceAssertion(statement, source, name) {
			return fmt.Errorf("Rust acceptance requires direct equality assertions against independent expected values")
		}
	}
	return nil
}

func rustAcceptanceResultBinding(node *treesitter.Node, source []byte) (string, error) {
	invalid := fmt.Errorf("Rust acceptance must first bind one literal-input run to its observed result")
	if node.Kind() != "let_declaration" || node.ChildByFieldName("alternative") != nil {
		return "", invalid
	}
	pattern, value := node.ChildByFieldName("pattern"), node.ChildByFieldName("value")
	if pattern == nil || pattern.Kind() != "identifier" || value == nil || value.Kind() != "call_expression" {
		return "", invalid
	}
	name := rustNodeText(source, pattern)
	function := value.ChildByFieldName("function")
	arguments := rustAcceptanceChildren(value.ChildByFieldName("arguments"))
	if name == "run" || name == "assert_eq" || function == nil || function.Kind() != "identifier" || rustNodeText(source, function) != "run" || len(arguments) != 2 {
		return "", invalid
	}
	for _, argument := range arguments {
		if !rustAcceptanceLiteral(argument, source) {
			return "", invalid
		}
	}
	return name, nil
}

func rustAcceptanceAssertion(node *treesitter.Node, source []byte, resultName string) bool {
	if node.Kind() == "expression_statement" {
		children := rustAcceptanceChildren(node)
		if len(children) != 1 {
			return false
		}
		node = children[0]
	}
	if node.Kind() != "macro_invocation" || rustNodeText(source, node.ChildByFieldName("macro")) != "assert_eq" {
		return false
	}
	arguments, argumentSource, closeTree, err := rustAcceptanceMacroExpression(node, source, "observe(", ")")
	if err != nil {
		return false
	}
	defer closeTree()
	if arguments.Kind() != "call_expression" {
		return false
	}
	values := rustAcceptanceChildren(arguments.ChildByFieldName("arguments"))
	if len(values) < 2 || len(values) > 3 || !rustAcceptanceObservedValue(values[0], argumentSource, resultName) || !rustAcceptanceLiteral(values[1], argumentSource) {
		return false
	}
	return len(values) == 2 || values[2].Kind() == "string_literal" || values[2].Kind() == "raw_string_literal"
}

func rustAcceptanceObservedValue(node *treesitter.Node, source []byte, name string) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier":
		return rustNodeText(source, node) == name
	case "field_expression", "reference_expression":
		return rustAcceptanceObservedValue(node.ChildByFieldName("value"), source, name)
	case "parenthesized_expression":
		children := rustAcceptanceChildren(node)
		return len(children) == 1 && rustAcceptanceObservedValue(children[0], source, name)
	case "index_expression":
		children := rustAcceptanceChildren(node)
		return len(children) == 2 && rustAcceptanceObservedValue(children[0], source, name) && rustAcceptanceLiteral(children[1], source)
	case "call_expression":
		function := node.ChildByFieldName("function")
		if function == nil || function.Kind() != "field_expression" || !rustAcceptanceObservedValue(function.ChildByFieldName("value"), source, name) {
			return false
		}
		arguments := rustAcceptanceChildren(node.ChildByFieldName("arguments"))
		switch rustNodeText(source, function.ChildByFieldName("field")) {
		case "as_str", "len", "is_empty", "clone", "to_string":
			return len(arguments) == 0
		case "get":
			return len(arguments) == 1 && rustAcceptanceLiteral(arguments[0], source)
		}
	}
	return false
}
