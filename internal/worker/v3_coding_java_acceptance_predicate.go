package worker

import (
	"fmt"
	"strconv"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func javaValidateAcceptanceCondition(
	source []byte,
	condition *treesitter.Node,
	resultName string,
) error {
	predicate := javaUnwrapParenthesizedExpression(condition)
	if predicate == nil {
		return fmt.Errorf("Java acceptance condition is empty")
	}
	if predicate.Kind() == "unary_expression" {
		operator := predicate.ChildByFieldName("operator")
		if operator == nil || javaNodeText(operator, source) != "!" {
			return fmt.Errorf("Java acceptance condition uses an unsupported wrapper")
		}
		predicate = javaUnwrapParenthesizedExpression(predicate.ChildByFieldName("operand"))
	}
	if predicate == nil || predicate.Kind() != "method_invocation" {
		return fmt.Errorf("Java acceptance condition must be one expected equals result predicate")
	}
	name := predicate.ChildByFieldName("name")
	expected := predicate.ChildByFieldName("object")
	arguments := javaCallArguments(predicate)
	if name == nil || javaNodeText(name, source) != "equals" || expected == nil ||
		len(arguments) != 1 {
		return fmt.Errorf("Java acceptance condition must be one expected equals result predicate")
	}
	observation := javaUnwrapParenthesizedExpression(arguments[0])
	if !javaResultGetInvocation(source, observation, resultName) {
		return fmt.Errorf("Java acceptance condition must directly observe %s through get", resultName)
	}
	if !javaAcceptanceExpectedExpression(source, expected) {
		return fmt.Errorf("Java acceptance expected expression is not one bounded value")
	}
	resultReferences := 0
	validReferences := true
	javaWalkTree(condition, func(node *treesitter.Node) {
		if javaResultGetInvocation(source, node, resultName) {
			resultReferences++
		}
		if node.Kind() == "identifier" && javaNodeText(node, source) == resultName &&
			!javaResultObservationReceiver(source, node, resultName) {
			validReferences = false
		}
	})
	if resultReferences != 1 || !validReferences {
		return fmt.Errorf("Java acceptance condition requires exactly one direct %s field observation", resultName)
	}
	return nil
}

func javaResultGetInvocation(source []byte, node *treesitter.Node, resultName string) bool {
	if !javaExactMethodInvocation(source, node, resultName, "get") {
		return false
	}
	arguments := javaCallArguments(node)
	if len(arguments) != 1 || arguments[0].Kind() != "string_literal" {
		return false
	}
	key, err := strconv.Unquote(javaNodeText(arguments[0], source))
	if err != nil {
		return false
	}
	switch key {
	case "output", "error", "exitCode", "state":
		return true
	default:
		return false
	}
}

func javaResultObservationReceiver(source []byte, node *treesitter.Node, resultName string) bool {
	parent := node.Parent()
	if parent == nil || parent.Kind() != "method_invocation" {
		return false
	}
	object := parent.ChildByFieldName("object")
	return object != nil && object.Id() == node.Id() &&
		javaResultGetInvocation(source, parent, resultName)
}

func javaAcceptanceExpectedExpression(source []byte, node *treesitter.Node) bool {
	node = javaUnwrapParenthesizedExpression(node)
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "string_literal", "character_literal", "decimal_integer_literal",
		"hex_integer_literal", "octal_integer_literal", "binary_integer_literal",
		"decimal_floating_point_literal", "hex_floating_point_literal":
		return true
	case "unary_expression":
		operator := node.ChildByFieldName("operator")
		operand := javaUnwrapParenthesizedExpression(node.ChildByFieldName("operand"))
		if operator == nil || operand == nil {
			return false
		}
		value := javaNodeText(operator, source)
		return (value == "+" || value == "-") && javaAcceptanceNumericLiteral(operand)
	case "method_invocation":
		return javaAcceptanceExpectedConstructor(source, node)
	default:
		return false
	}
}

func javaAcceptanceNumericLiteral(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal",
		"binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal":
		return true
	default:
		return false
	}
}

func javaAcceptanceExpectedConstructor(source []byte, node *treesitter.Node) bool {
	object := node.ChildByFieldName("object")
	name := node.ChildByFieldName("name")
	if object == nil || object.Kind() != "identifier" || name == nil {
		return false
	}
	owner, method := javaNodeText(object, source), javaNodeText(name, source)
	allowed := method == "of" && (owner == "Map" || owner == "List") ||
		method == "valueOf" && (owner == "String" || owner == "Integer" ||
			owner == "Long" || owner == "Double")
	if !allowed {
		return false
	}
	for _, argument := range javaCallArguments(node) {
		if !javaAcceptanceExpectedExpression(source, argument) {
			return false
		}
	}
	return true
}
