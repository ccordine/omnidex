package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func inspectRustAcceptance(
	source []byte,
	implementationName string,
	fixtureName string,
) (bool, int, error) {
	root, closeTree, err := parseRustAuthorityTree(source)
	if err != nil {
		return false, 0, fmt.Errorf("Rust acceptance source is not parseable: %w", err)
	}
	defer closeTree()
	var functionBody *treesitter.Node
	resultNames := make(map[string]struct{})
	functionItems := 0
	implementationCalls := 0
	forbidden := ""
	walkRustTree(root, func(node *treesitter.Node) {
		switch node.Kind() {
		case "function_item":
			functionItems++
			functionBody = node.ChildByFieldName("body")
		case "macro_definition", "use_declaration", "extern_crate_declaration", "unsafe_block":
			forbidden = node.Kind()
		case "identifier":
			if rustAcceptanceForbiddenIdentifier(rustNodeText(source, node)) {
				forbidden = rustNodeText(source, node)
			}
		case "call_expression":
			callable := node.ChildByFieldName("function")
			if callable != nil && rustNodeText(source, callable) == implementationName {
				implementationCalls++
			}
		case "let_declaration":
			pattern := node.ChildByFieldName("pattern")
			value := node.ChildByFieldName("value")
			if pattern == nil || pattern.Kind() != "identifier" {
				return
			}
			name := rustNodeText(source, pattern)
			if name == implementationName || rustAcceptanceAssertionMacro(name) {
				forbidden = "redefined " + name
			}
			if functionBody != nil && rustAcceptanceDirectChild(node, functionBody) &&
				rustAcceptanceCallUsesFixture(source, value, implementationName, fixtureName) {
				resultNames[name] = struct{}{}
			}
		}
	})
	if forbidden != "" {
		return false, 0, fmt.Errorf("Rust acceptance uses forbidden authority %s", forbidden)
	}
	if functionItems != 1 || functionBody == nil {
		return false, 0, fmt.Errorf("Rust acceptance contains %d function declarations", functionItems)
	}
	if implementationCalls != 1 || len(resultNames) != 1 {
		return false, 0, nil
	}
	assertions := make(map[string]struct{})
	validStructure := true
	for index := uint(0); index < functionBody.NamedChildCount(); index++ {
		statement := functionBody.NamedChild(index)
		switch statement.Kind() {
		case "let_declaration":
			continue
		case "expression_statement":
			if statement.NamedChildCount() != 1 {
				validStructure = false
				continue
			}
			key, meaningful := rustAcceptanceMeaningfulAssertion(
				source, statement.NamedChild(0), resultNames,
			)
			if !meaningful {
				validStructure = false
				continue
			}
			assertions[key] = struct{}{}
		default:
			validStructure = false
		}
	}
	if !validStructure {
		return false, 0, nil
	}
	return true, len(assertions), nil
}

func rustAcceptanceDirectChild(node, body *treesitter.Node) bool {
	return node != nil && body != nil && node.Parent() != nil && node.Parent().Id() == body.Id()
}

func rustAcceptanceCallUsesFixture(
	source []byte,
	node *treesitter.Node,
	implementationName string,
	fixtureName string,
) bool {
	if node == nil || node.Kind() != "call_expression" {
		return false
	}
	callable := node.ChildByFieldName("function")
	arguments := node.ChildByFieldName("arguments")
	return callable != nil && rustNodeText(source, callable) == implementationName &&
		arguments != nil && rustSubtreeCalls(source, arguments, fixtureName)
}

func rustSubtreeCalls(source []byte, node *treesitter.Node, name string) bool {
	called := false
	walkRustTree(node, func(candidate *treesitter.Node) {
		if candidate.Kind() != "call_expression" {
			return
		}
		function := candidate.ChildByFieldName("function")
		if function != nil && rustNodeText(source, function) == name {
			called = true
		}
	})
	return called
}

func rustAcceptanceAssertionMacro(name string) bool {
	switch name {
	case "assert", "assert_eq", "assert_ne":
		return true
	default:
		return false
	}
}

func rustAcceptanceForbiddenIdentifier(name string) bool {
	switch name {
	case "std", "fs", "File", "OpenOptions", "Command", "TcpStream", "UdpSocket",
		"env", "process", "include", "include_str", "include_bytes":
		return true
	default:
		return false
	}
}
