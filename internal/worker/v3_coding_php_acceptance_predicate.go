package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func phpAcceptanceConditionFingerprint(
	source []byte,
	node *treesitter.Node,
	resultName string,
) (string, error) {
	predicate := phpAcceptanceUnwrapParentheses(node)
	if predicate == nil || predicate.Kind() != "binary_expression" {
		return "", fmt.Errorf("PHP acceptance condition must be one direct result comparison")
	}
	operator := phpNodeText(source, predicate.ChildByFieldName("operator"))
	if !phpAcceptanceComparisonOperator(operator) {
		return "", fmt.Errorf("PHP acceptance condition uses forbidden operator %s", operator)
	}
	left, right := predicate.ChildByFieldName("left"), predicate.ChildByFieldName("right")
	leftObservation := phpAcceptanceExactResultObservation(source, left, resultName)
	rightObservation := phpAcceptanceExactResultObservation(source, right, resultName)
	if leftObservation == rightObservation {
		return "", fmt.Errorf(
			"PHP acceptance comparison must have one exact TaskResult field observation",
		)
	}
	expected := left
	if leftObservation {
		expected = right
	}
	if err := phpAcceptanceExpectedExpression(source, expected, resultName); err != nil {
		return "", err
	}
	if phpAcceptanceResultObservationCount(source, predicate, resultName) != 1 {
		return "", fmt.Errorf("PHP acceptance condition must read exactly one TaskResult field")
	}
	return phpAcceptanceNodeFingerprint(source, predicate), nil
}

func phpAcceptanceComparisonOperator(operator string) bool {
	switch operator {
	case "==", "===", "!=", "!==", "<>", "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func phpAcceptanceExactResultObservation(
	source []byte,
	node *treesitter.Node,
	resultName string,
) bool {
	node = phpAcceptanceUnwrapParentheses(node)
	if node == nil || node.Kind() != "member_access_expression" {
		return false
	}
	object, property := node.ChildByFieldName("object"), node.ChildByFieldName("name")
	return object != nil && object.Kind() == "variable_name" &&
		phpNodeText(source, object) == resultName && property != nil &&
		property.Kind() == "name" && phpTaskResultField(phpNodeText(source, property))
}

func phpAcceptanceExpectedExpression(
	source []byte,
	node *treesitter.Node,
	resultName string,
) error {
	var failure error
	walkPHPTree(node, func(candidate *treesitter.Node) {
		if failure != nil {
			return
		}
		switch candidate.Kind() {
		case "cast_expression":
			failure = fmt.Errorf("PHP acceptance expected expression uses a cast")
		case "boolean":
			failure = fmt.Errorf("PHP acceptance expected expression uses a boolean literal")
		case "conditional_expression":
			failure = fmt.Errorf("PHP acceptance expected expression uses a conditional")
		case "binary_expression":
			if phpAcceptanceTruthForcingOperator(
				strings.ToLower(phpNodeText(source, candidate.ChildByFieldName("operator"))),
			) {
				failure = fmt.Errorf("PHP acceptance expected expression composes a truth value")
			}
		case "unary_op_expression":
			operator := phpAcceptanceUnaryOperator(source, candidate)
			if operator == "!" || strings.EqualFold(operator, "not") || operator == "~" {
				failure = fmt.Errorf("PHP acceptance expected expression forces a truth value")
			}
		case "variable_name":
			if phpNodeText(source, candidate) == resultName {
				failure = fmt.Errorf("PHP acceptance expected expression depends on the feature result")
			}
		case "member_access_expression", "nullsafe_member_access_expression":
			property := candidate.ChildByFieldName("name")
			if property != nil && phpTaskResultField(phpNodeText(source, property)) {
				failure = fmt.Errorf("PHP acceptance expected expression reads a detached result field")
			}
		}
	})
	return failure
}

func phpAcceptanceTruthForcingOperator(operator string) bool {
	switch operator {
	case "|", "&", "^", "<<", ">>", "||", "&&", "or", "and", "xor",
		"==", "===", "!=", "!==", "<>", "<", ">", "<=", ">=", "<=>", "instanceof":
		return true
	default:
		return false
	}
}

func phpAcceptanceUnaryOperator(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child != nil && !child.IsNamed() {
			return strings.TrimSpace(phpNodeText(source, child))
		}
	}
	return ""
}

func phpAcceptanceResultObservationCount(
	source []byte,
	node *treesitter.Node,
	resultName string,
) int {
	count := 0
	walkPHPTree(node, func(candidate *treesitter.Node) {
		if candidate.Kind() != "member_access_expression" {
			return
		}
		object, property := candidate.ChildByFieldName("object"), candidate.ChildByFieldName("name")
		if object != nil && object.Kind() == "variable_name" &&
			phpNodeText(source, object) == resultName && property != nil &&
			phpTaskResultField(phpNodeText(source, property)) {
			count++
		}
	})
	return count
}

func phpAcceptanceUnwrapParentheses(node *treesitter.Node) *treesitter.Node {
	for node != nil && node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}
	return node
}

func phpTaskResultField(name string) bool {
	switch name {
	case "output", "error", "state":
		return true
	default:
		return false
	}
}

func phpAcceptanceNodeFingerprint(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		return phpAcceptanceNodeFingerprint(source, node.NamedChild(0))
	}
	if node.Kind() == "binary_expression" {
		left := phpAcceptanceNodeFingerprint(source, node.ChildByFieldName("left"))
		right := phpAcceptanceNodeFingerprint(source, node.ChildByFieldName("right"))
		operator := phpNodeText(source, node.ChildByFieldName("operator"))
		switch operator {
		case "==", "===", "!=", "!==", "<>":
			if operator == "<>" {
				operator = "!="
			}
			if right < left {
				left, right = right, left
			}
		case ">":
			operator, left, right = "<", right, left
		case ">=":
			operator, left, right = "<=", right, left
		}
		return "(binary_expression:" + operator + left + right + ")"
	}
	var fingerprint strings.Builder
	fingerprint.WriteByte('(')
	fingerprint.WriteString(node.Kind())
	if node.ChildCount() == 0 {
		fingerprint.WriteByte(':')
		fingerprint.WriteString(phpNodeText(source, node))
	} else {
		for index := uint(0); index < node.ChildCount(); index++ {
			fingerprint.WriteString(phpAcceptanceNodeFingerprint(source, node.Child(index)))
		}
	}
	fingerprint.WriteByte(')')
	return fingerprint.String()
}
