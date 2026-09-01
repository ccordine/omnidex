package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

const directCodingBrowserExecutionGrammar = "one direct fireEvent call, one direct expect assertion with a permitted matcher, or one awaited waitFor callback containing only direct expect assertions"

func (validator *directCodingBrowserAcceptanceQueryValidator) validateDirectExecutionBody(
	body *treesitter.Node,
) error {
	for index := uint(0); index < body.NamedChildCount(); index++ {
		statement := body.NamedChild(index)
		if statement == nil || statement.Kind() != "expression_statement" ||
			statement.NamedChildCount() != 1 {
			return fmt.Errorf(
				"browser acceptance verification body permits only %s",
				directCodingBrowserExecutionGrammar,
			)
		}
		if err := validator.validateDirectExecutionExpression(statement.NamedChild(0)); err != nil {
			return err
		}
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateDirectExecutionExpression(
	expression *treesitter.Node,
) error {
	expression = directCodingBrowserUnwrapExecutionExpression(expression)
	if expression == nil {
		return validator.directExecutionGrammarError()
	}
	if expression.Kind() == "call_expression" {
		callee := expression.ChildByFieldName("function")
		if validator.allowedDirectFireEventCallee(callee) {
			return nil
		}
		if expectCall, matcher, direct := validator.directRootExpectation(expression); direct {
			return validator.validateDirectExpectationMatcher(expectCall, matcher)
		}
		return validator.directExecutionGrammarError()
	}
	if expression.Kind() == "await_expression" {
		return validator.validateAwaitedWaitForExecution(expression)
	}
	return validator.directExecutionGrammarError()
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateAwaitedWaitForExecution(
	await *treesitter.Node,
) error {
	if await == nil || await.NamedChildCount() != 1 {
		return validator.directExecutionGrammarError()
	}
	call := directCodingBrowserUnwrapExecutionExpression(await.NamedChild(0))
	if call == nil || call.Kind() != "call_expression" {
		return validator.directExecutionGrammarError()
	}
	callee := call.ChildByFieldName("function")
	if callee == nil || callee.Kind() != "identifier" || validator.text(callee) != "waitFor" {
		return validator.directExecutionGrammarError()
	}
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return validator.directExecutionGrammarError()
	}
	callback := arguments.NamedChild(0)
	if callback == nil ||
		(callback.Kind() != "arrow_function" && callback.Kind() != "function_expression") {
		return validator.directExecutionGrammarError()
	}
	body := callback.ChildByFieldName("body")
	if body == nil {
		return validator.directExecutionGrammarError()
	}
	if body.Kind() != "statement_block" {
		expectCall, matcher, direct := validator.directRootExpectation(
			directCodingBrowserUnwrapExecutionExpression(body),
		)
		if !direct {
			return validator.directExecutionGrammarError()
		}
		return validator.validateDirectExpectationMatcher(expectCall, matcher)
	}
	if !directCodingBrowserFlatExpressionStatements(body) {
		return validator.directExecutionGrammarError()
	}
	for index := uint(0); index < body.NamedChildCount(); index++ {
		statement := body.NamedChild(index)
		if statement == nil || statement.NamedChildCount() != 1 {
			return validator.directExecutionGrammarError()
		}
		expectCall, matcher, direct := validator.directRootExpectation(
			directCodingBrowserUnwrapExecutionExpression(statement.NamedChild(0)),
		)
		if !direct {
			return validator.directExecutionGrammarError()
		}
		if err := validator.validateDirectExpectationMatcher(expectCall, matcher); err != nil {
			return err
		}
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directRootExpectation(
	expression *treesitter.Node,
) (*treesitter.Node, directCodingBrowserExpectationMatcher, bool) {
	if expression == nil || expression.Kind() != "call_expression" {
		return nil, directCodingBrowserExpectationMatcher{}, false
	}
	member := expression.ChildByFieldName("function")
	if member == nil || member.Kind() != "member_expression" ||
		directCodingBrowserNodeHasChildKind(member, "optional_chain") {
		return nil, directCodingBrowserExpectationMatcher{}, false
	}
	object := member.ChildByFieldName("object")
	if object == nil {
		return nil, directCodingBrowserExpectationMatcher{}, false
	}
	expectCall := object
	if object.Kind() == "member_expression" {
		property := object.ChildByFieldName("property")
		if property == nil || validator.text(property) != "not" ||
			directCodingBrowserNodeHasChildKind(object, "optional_chain") {
			return nil, directCodingBrowserExpectationMatcher{}, false
		}
		expectCall = object.ChildByFieldName("object")
	}
	if !validator.directExpectCall(expectCall) {
		return nil, directCodingBrowserExpectationMatcher{}, false
	}
	matcher, valid := validator.directExpectationMatcher(expectCall)
	if !valid || matcher.arguments == nil ||
		!directCodingBrowserSameNode(matcher.arguments.Parent(), expression) {
		return nil, directCodingBrowserExpectationMatcher{}, false
	}
	return expectCall, matcher, true
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directExpectCall(
	call *treesitter.Node,
) bool {
	if call == nil || call.Kind() != "call_expression" {
		return false
	}
	callee := call.ChildByFieldName("function")
	return callee != nil && callee.Kind() == "identifier" && validator.text(callee) == "expect"
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateDirectExpectationMatcher(
	expectCall *treesitter.Node,
	matcher directCodingBrowserExpectationMatcher,
) error {
	arguments := expectCall.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return validator.directExecutionGrammarError()
	}
	selection := directCodingBrowserUnwrapOutcomeSelection(arguments.NamedChild(0))
	if selection == nil {
		return validator.directExecutionGrammarError()
	}
	methodName, grounded := validator.screenQuerySelections[selection.Id()]
	if !grounded {
		return validator.directExecutionGrammarError()
	}
	if _, output := validator.outputSelections[selection.Id()]; output {
		if directCodingBrowserPresenceMatcher(matcher) ||
			directCodingBrowserOutputOutcomeMatcher(matcher, validator) {
			return nil
		}
		return validator.directMatcherGrammarError(matcher.name)
	}
	if methodName == "getByText" || methodName == "findByText" {
		if directCodingBrowserPresenceMatcher(matcher) {
			return nil
		}
		return validator.directMatcherGrammarError(matcher.name)
	}
	method, exists := directCodingBrowserScreenQueryMethods[methodName]
	if !exists || !method.role {
		return validator.directMatcherGrammarError(matcher.name)
	}
	control, grounded := validator.roleSelections[selection.Id()]
	if !grounded {
		return validator.directExecutionGrammarError()
	}
	if directCodingBrowserPresenceMatcher(matcher) ||
		directCodingBrowserRoleOutcomeMatcher(matcher, control, validator) {
		return nil
	}
	return validator.directMatcherGrammarError(matcher.name)
}

func directCodingBrowserUnwrapExecutionExpression(node *treesitter.Node) *treesitter.Node {
	current := node
	for current != nil && current.Kind() == "parenthesized_expression" {
		if current.NamedChildCount() != 1 {
			return nil
		}
		current = current.NamedChild(0)
	}
	return current
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directExecutionGrammarError() error {
	return fmt.Errorf(
		"browser acceptance execution expression must be %s",
		directCodingBrowserExecutionGrammar,
	)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directMatcherGrammarError(
	matcher string,
) error {
	return fmt.Errorf(
		"browser acceptance direct expect matcher %s is unsupported or has non-static side-effect-capable arguments",
		matcher,
	)
}
