package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (validator *directCodingBrowserAcceptanceQueryValidator) validateScreenQueryConsumer(
	call *treesitter.Node,
	methodName string,
	method directCodingBrowserScreenQueryMethod,
) error {
	selection, err := validator.screenQuerySelection(call, methodName, method)
	if err != nil {
		return err
	}
	querySelection := directCodingBrowserUnwrapOutcomeSelection(selection)
	if querySelection == nil {
		return directCodingBrowserScreenQueryConsumerError(methodName)
	}
	selection = directCodingBrowserOutermostParentheses(selection)
	arguments := selection.Parent()
	if arguments == nil || arguments.Kind() != "arguments" ||
		arguments.NamedChildCount() == 0 ||
		!directCodingBrowserSameNode(arguments.NamedChild(0), selection) {
		return directCodingBrowserScreenQueryConsumerError(methodName)
	}
	consumer := arguments.Parent()
	if consumer == nil || consumer.Kind() != "call_expression" {
		return directCodingBrowserScreenQueryConsumerError(methodName)
	}
	callee := consumer.ChildByFieldName("function")
	if callee != nil && callee.Kind() == "identifier" && validator.text(callee) == "expect" {
		if arguments.NamedChildCount() != 1 {
			return directCodingBrowserScreenQueryConsumerError(methodName)
		}
		validator.screenQuerySelections[querySelection.Id()] = methodName
		return nil
	}
	if validator.allowedDirectFireEventCallee(callee) {
		validator.screenQuerySelections[querySelection.Id()] = methodName
		return nil
	}
	return directCodingBrowserScreenQueryConsumerError(methodName)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) screenQuerySelection(
	call *treesitter.Node,
	methodName string,
	method directCodingBrowserScreenQueryMethod,
) (*treesitter.Node, error) {
	selection := call
	if method.asynchronous {
		selection = directCodingBrowserOutermostParentheses(selection)
		await := selection.Parent()
		if await == nil || await.Kind() != "await_expression" {
			return nil, fmt.Errorf(
				"browser acceptance query %s must be explicitly awaited",
				methodName,
			)
		}
		selection = await
	}
	selection = directCodingBrowserOutermostParentheses(selection)
	if method.plural {
		subscript := selection.Parent()
		if subscript == nil || subscript.Kind() != "subscript_expression" ||
			!directCodingBrowserSameNode(subscript.ChildByFieldName("object"), selection) {
			return nil, fmt.Errorf(
				"browser acceptance plural role query %s requires an exact literal index",
				methodName,
			)
		}
		selection = subscript
	}
	return selection, nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) allowedDirectFireEventCallee(
	callee *treesitter.Node,
) bool {
	if callee == nil || callee.Kind() != "member_expression" ||
		directCodingBrowserNodeHasChildKind(callee, "optional_chain") {
		return false
	}
	object := callee.ChildByFieldName("object")
	property := callee.ChildByFieldName("property")
	if object == nil || property == nil || object.Kind() != "identifier" ||
		validator.text(object) != "fireEvent" {
		return false
	}
	_, allowed := directCodingBrowserFireEventMethods[validator.text(property)]
	return allowed
}

func directCodingBrowserOutermostParentheses(node *treesitter.Node) *treesitter.Node {
	current := node
	for current != nil && current.Parent() != nil &&
		current.Parent().Kind() == "parenthesized_expression" {
		current = current.Parent()
	}
	return current
}

func directCodingBrowserScreenQueryConsumerError(methodName string) error {
	return fmt.Errorf(
		"browser acceptance screen query %s must be consumed directly by expect(...) or as the first target of an allowed fireEvent call",
		methodName,
	)
}
