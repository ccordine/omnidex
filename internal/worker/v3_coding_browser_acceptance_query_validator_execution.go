package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserAcceptanceAuthorities = map[string]struct{}{
	"screen": {}, "expect": {}, "fireEvent": {}, "waitFor": {},
}

func rejectDirectCodingBrowserAcceptanceAuthorityShadowing(
	root *treesitter.Node,
	source []byte,
) error {
	_, err := collectJavaScriptLexicalBindings(
		root,
		source,
		directCodingBrowserAcceptanceAuthorities,
		map[string]struct{}{},
	)
	if err != nil {
		return fmt.Errorf("browser acceptance authority binding: %w", err)
	}
	return nil
}

func directCodingBrowserAcceptanceExecutionRoot(
	root *treesitter.Node,
) (*treesitter.Node, error) {
	functions := make([]*treesitter.Node, 0, 1)
	var collect func(*treesitter.Node, bool)
	collect = func(node *treesitter.Node, insideFunction bool) {
		if node == nil {
			return
		}
		function := javaScriptFunctionScopeKind(node.Kind())
		if function && !insideFunction {
			functions = append(functions, node)
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			collect(node.NamedChild(index), insideFunction || function)
		}
	}
	collect(root, false)
	if len(functions) != 1 || functions[0].ChildByFieldName("body") == nil {
		return nil, fmt.Errorf("browser acceptance requires one exact executable verification function")
	}
	return functions[0], nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateFlatExecutionBodies() error {
	body := validator.rootFunction.ChildByFieldName("body")
	if !directCodingBrowserFlatExpressionStatements(body) {
		return fmt.Errorf("browser acceptance verification body must be one flat sequence of expression statements")
	}
	return validator.validateDirectExecutionBody(body)
}

func directCodingBrowserFlatExpressionStatements(body *treesitter.Node) bool {
	if body == nil || body.Kind() != "statement_block" || body.NamedChildCount() == 0 {
		return false
	}
	for index := uint(0); index < body.NamedChildCount(); index++ {
		child := body.NamedChild(index)
		if child == nil || child.Kind() != "expression_statement" {
			return false
		}
	}
	return true
}

func (validator *directCodingBrowserAcceptanceQueryValidator) requireExecuted(
	node *treesitter.Node,
	allowWaitForCallback bool,
) error {
	function := directCodingBrowserNearestFunction(node)
	if function == nil {
		return fmt.Errorf("browser acceptance query or assertion is outside the verification function")
	}
	if directCodingBrowserSameNode(function, validator.rootFunction) {
		body := function.ChildByFieldName("body")
		if body == nil || node.StartByte() < body.StartByte() || node.EndByte() > body.EndByte() {
			return fmt.Errorf("browser acceptance query or assertion is outside the executable function body")
		}
		return validator.rejectConditionalExecution(node, function)
	}
	if allowWaitForCallback {
		waitForCall, direct := validator.directWaitForCallback(function)
		if direct {
			if err := validator.rejectConditionalExecution(node, function); err != nil {
				return err
			}
			return validator.requireExecuted(waitForCall, false)
		}
	}
	return fmt.Errorf("browser acceptance rejects queries or assertions in nested or dead closures")
}

func (validator *directCodingBrowserAcceptanceQueryValidator) rejectConditionalExecution(
	node *treesitter.Node,
	boundary *treesitter.Node,
) error {
	for parent := node.Parent(); parent != nil && !directCodingBrowserSameNode(parent, boundary); parent = parent.Parent() {
		switch parent.Kind() {
		case "if_statement", "switch_statement", "switch_case", "switch_default",
			"for_statement", "for_in_statement", "while_statement", "do_statement",
			"try_statement", "catch_clause", "finally_clause", "with_statement",
			"ternary_expression", "conditional_expression":
			return fmt.Errorf("browser acceptance rejects queries or assertions in conditional or control-flow branches")
		case "binary_expression":
			operator := parent.ChildByFieldName("operator")
			if operator == nil || validator.text(operator) == "&&" ||
				validator.text(operator) == "||" || validator.text(operator) == "??" {
				return fmt.Errorf("browser acceptance rejects queries or assertions in conditional expressions")
			}
		}
	}
	return nil
}

func directCodingBrowserNearestFunction(node *treesitter.Node) *treesitter.Node {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if javaScriptFunctionScopeKind(parent.Kind()) {
			return parent
		}
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) directWaitForCallback(
	callback *treesitter.Node,
) (*treesitter.Node, bool) {
	if callback == nil || (callback.Kind() != "arrow_function" && callback.Kind() != "function_expression") {
		return nil, false
	}
	arguments := callback.Parent()
	if arguments == nil || arguments.Kind() != "arguments" || arguments.NamedChildCount() != 1 ||
		!directCodingBrowserSameNode(arguments.NamedChild(0), callback) {
		return nil, false
	}
	call := arguments.Parent()
	if call == nil || call.Kind() != "call_expression" {
		return nil, false
	}
	callee := call.ChildByFieldName("function")
	return call, callee != nil && callee.Kind() == "identifier" && validator.text(callee) == "waitFor"
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateExpectCall(
	call *treesitter.Node,
) error {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return fmt.Errorf("browser acceptance expect requires exactly one asserted value")
	}
	if err := validator.requireExecuted(call, true); err != nil {
		return err
	}
	if !directCodingBrowserExpectHasMatcherCall(call) {
		return fmt.Errorf("browser acceptance expect must invoke an assertion matcher")
	}
	if directCodingBrowserSubtreeHasPermittedScreenQuery(arguments, validator) {
		validator.executedAsserts++
		validator.recordOutcomeAssertion(call)
	}
	return nil
}

func directCodingBrowserExpectHasMatcherCall(expectCall *treesitter.Node) bool {
	current := expectCall
	for current.Parent() != nil && current.Parent().Kind() == "member_expression" &&
		directCodingBrowserSameNode(current.Parent().ChildByFieldName("object"), current) {
		current = current.Parent()
	}
	parent := current.Parent()
	return parent != nil && parent.Kind() == "call_expression" &&
		directCodingBrowserSameNode(parent.ChildByFieldName("function"), current)
}

func directCodingBrowserSubtreeHasPermittedScreenQuery(
	node *treesitter.Node,
	validator *directCodingBrowserAcceptanceQueryValidator,
) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "member_expression" {
		object := node.ChildByFieldName("object")
		property := node.ChildByFieldName("property")
		if object != nil && property != nil && object.Kind() == "identifier" &&
			validator.text(object) == "screen" {
			method, exists := directCodingBrowserScreenQueryMethods[validator.text(property)]
			if exists && method.supported {
				return true
			}
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if directCodingBrowserSubtreeHasPermittedScreenQuery(node.NamedChild(index), validator) {
			return true
		}
	}
	return false
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateWaitForCall(
	call *treesitter.Node,
) error {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return fmt.Errorf("browser acceptance waitFor requires one direct callback")
	}
	callback := arguments.NamedChild(0)
	if callback == nil || (callback.Kind() != "arrow_function" && callback.Kind() != "function_expression") {
		return fmt.Errorf("browser acceptance waitFor requires one parameterless direct callback")
	}
	parameters := callback.ChildByFieldName("parameters")
	if callback.ChildByFieldName("parameter") != nil ||
		(parameters != nil && parameters.NamedChildCount() != 0) {
		return fmt.Errorf("browser acceptance waitFor requires one parameterless direct callback")
	}
	body := callback.ChildByFieldName("body")
	if body == nil || (body.Kind() == "statement_block" && !directCodingBrowserFlatExpressionStatements(body)) {
		return fmt.Errorf("browser acceptance waitFor callback must be one direct expression or flat expression sequence")
	}
	if err := validator.requireExecuted(call, false); err != nil {
		return err
	}
	if !directCodingBrowserCallIsAwaited(call) {
		return fmt.Errorf("browser acceptance waitFor must be explicitly awaited")
	}
	return nil
}
