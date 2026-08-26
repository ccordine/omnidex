package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func rustAcceptanceMeaningfulAssertion(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) (string, bool) {
	if node == nil || node.Kind() != "macro_invocation" {
		return "", false
	}
	macro := node.ChildByFieldName("macro")
	if macro == nil || !rustAcceptanceAssertionMacro(rustNodeText(source, macro)) {
		return "", false
	}
	payload := rustAcceptanceMacroPayload(rustNodeText(source, node))
	arguments, ok := rustAcceptanceSplitArguments(payload)
	if !ok || len(arguments) == 0 {
		return "", false
	}
	name := rustNodeText(source, macro)
	switch name {
	case "assert":
		predicate, valid := rustAcceptanceParsedPredicate(arguments[0], resultNames)
		return name + ":" + predicate, valid
	case "assert_eq", "assert_ne":
		if len(arguments) < 2 {
			return "", false
		}
		actual, observed := rustAcceptanceParsedObservation(arguments[0], resultNames)
		expected, independent := rustAcceptanceParsedExpected(arguments[1])
		if !observed || !independent {
			return "", false
		}
		return name + ":" + actual + "," + expected, true
	default:
		return "", false
	}
}

func rustAcceptanceParsedPredicate(
	expression string,
	resultNames map[string]struct{},
) (string, bool) {
	source, node, closeTree, ok := rustParseAcceptanceExpression(expression)
	if !ok {
		return "", false
	}
	defer closeTree()
	node = rustUnwrapParenthesizedExpression(node)
	if node == nil {
		return "", false
	}
	if node.Kind() == "binary_expression" {
		operator := node.ChildByFieldName("operator")
		left := rustUnwrapParenthesizedExpression(node.ChildByFieldName("left"))
		right := rustUnwrapParenthesizedExpression(node.ChildByFieldName("right"))
		if operator == nil || !rustAcceptanceComparisonOperator(rustNodeText(source, operator)) {
			return "", false
		}
		leftObserved := rustAcceptanceObservation(source, left, resultNames)
		rightObserved := rustAcceptanceObservation(source, right, resultNames)
		valid := leftObserved && !rightObserved && rustAcceptanceExpected(source, right) ||
			rightObserved && !leftObserved && rustAcceptanceExpected(source, left)
		return rustAcceptanceCanonical(source, node), valid
	}
	if rustAcceptanceTestObservation(source, node, resultNames) {
		return rustAcceptanceCanonical(source, node), true
	}
	return "", false
}

func rustAcceptanceParsedObservation(
	expression string,
	resultNames map[string]struct{},
) (string, bool) {
	source, node, closeTree, ok := rustParseAcceptanceExpression(expression)
	if !ok {
		return "", false
	}
	defer closeTree()
	node = rustUnwrapParenthesizedExpression(node)
	return rustAcceptanceCanonical(source, node),
		rustAcceptanceObservation(source, node, resultNames)
}

func rustAcceptanceParsedExpected(expression string) (string, bool) {
	source, node, closeTree, ok := rustParseAcceptanceExpression(expression)
	if !ok {
		return "", false
	}
	defer closeTree()
	node = rustUnwrapParenthesizedExpression(node)
	return rustAcceptanceCanonical(source, node), rustAcceptanceExpected(source, node)
}

func rustAcceptanceObservation(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) bool {
	node = rustUnwrapParenthesizedExpression(node)
	if rustAcceptanceDirectResultField(source, node, resultNames) {
		return true
	}
	receiver, method, arguments, ok := rustAcceptanceMethodCall(source, node)
	if !ok || !rustAcceptanceDirectResultField(source, receiver, resultNames) {
		return false
	}
	wantArguments := -1
	switch method {
	case "len":
		wantArguments = 0
	case "get":
		wantArguments = 1
	}
	return wantArguments >= 0 && len(arguments) == wantArguments &&
		rustAcceptanceExpectedArguments(source, arguments)
}

func rustAcceptanceTestObservation(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) bool {
	receiver, method, arguments, ok := rustAcceptanceMethodCall(source, node)
	if !ok || !rustAcceptanceDirectResultField(source, receiver, resultNames) {
		return false
	}
	wantArguments := -1
	switch method {
	case "is_empty":
		wantArguments = 0
	case "contains", "starts_with", "ends_with", "contains_key":
		wantArguments = 1
	}
	return wantArguments >= 0 && len(arguments) == wantArguments &&
		rustAcceptanceExpectedArguments(source, arguments)
}

func rustAcceptanceDirectResultField(
	source []byte,
	node *treesitter.Node,
	resultNames map[string]struct{},
) bool {
	if node == nil || node.Kind() != "field_expression" {
		return false
	}
	value, field := node.ChildByFieldName("value"), node.ChildByFieldName("field")
	if value == nil || value.Kind() != "identifier" || field == nil {
		return false
	}
	if _, exists := resultNames[rustNodeText(source, value)]; !exists {
		return false
	}
	switch rustNodeText(source, field) {
	case "output", "error", "exit_code", "state":
		return true
	default:
		return false
	}
}

func rustAcceptanceMethodCall(
	source []byte,
	node *treesitter.Node,
) (*treesitter.Node, string, []*treesitter.Node, bool) {
	if node == nil || node.Kind() != "call_expression" {
		return nil, "", nil, false
	}
	callable := node.ChildByFieldName("function")
	argumentList := node.ChildByFieldName("arguments")
	if callable == nil || callable.Kind() != "field_expression" || argumentList == nil {
		return nil, "", nil, false
	}
	receiver := callable.ChildByFieldName("value")
	method := callable.ChildByFieldName("field")
	if receiver == nil || method == nil {
		return nil, "", nil, false
	}
	arguments := make([]*treesitter.Node, 0, argumentList.NamedChildCount())
	for index := uint(0); index < argumentList.NamedChildCount(); index++ {
		arguments = append(arguments, argumentList.NamedChild(index))
	}
	return receiver, rustNodeText(source, method), arguments, true
}

func rustAcceptanceExpectedArguments(source []byte, nodes []*treesitter.Node) bool {
	for _, node := range nodes {
		if !rustAcceptanceExpected(source, node) {
			return false
		}
	}
	return true
}

func rustAcceptanceExpected(source []byte, node *treesitter.Node) bool {
	node = rustUnwrapParenthesizedExpression(node)
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "string_literal", "char_literal", "integer_literal", "float_literal":
		return true
	case "identifier":
		return rustNodeText(source, node) == "None"
	case "unary_expression":
		if node.NamedChildCount() != 1 {
			return false
		}
		operand := node.NamedChild(0)
		operator := strings.TrimSpace(string(source[node.StartByte():operand.StartByte()]))
		return operator == "-" && (operand.Kind() == "integer_literal" ||
			operand.Kind() == "float_literal")
	default:
		return false
	}
}

func rustAcceptanceComparisonOperator(operator string) bool {
	switch operator {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}
