package assemblyline

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type acceptanceObserverGrammar struct {
	trustedCalls      map[uintptr]struct{}
	allowedStatements map[uintptr]struct{}
	source            []byte
}

func analyzeAcceptanceObserverGrammar(
	declaration *treesitter.Node,
	source []byte,
) acceptanceObserverGrammar {
	grammar := acceptanceObserverGrammar{
		trustedCalls: make(map[uintptr]struct{}), allowedStatements: make(map[uintptr]struct{}),
		source: source,
	}
	body := declaration.ChildByFieldName("body")
	if body == nil {
		return grammar
	}
	for index := uint(0); index < body.NamedChildCount(); index++ {
		statement := body.NamedChild(index)
		if statement != nil && statement.Kind() != "comment" && grammar.allowStatement(statement) {
			grammar.allowedStatements[statement.Id()] = struct{}{}
		}
	}
	return grammar
}

func (grammar acceptanceObserverGrammar) allowStatement(statement *treesitter.Node) bool {
	if statement == nil || statement.Kind() != "expression_statement" || statement.NamedChildCount() != 1 {
		return false
	}
	expression := statement.NamedChild(0)
	if expression == nil {
		return false
	}
	awaited := false
	if expression.Kind() == "await_expression" && expression.NamedChildCount() == 1 {
		awaited = true
		expression = expression.NamedChild(0)
	}
	if expression == nil || expression.Kind() != "call_expression" {
		return false
	}
	operation := acceptanceCallOperation(expression, grammar.source)
	switch {
	case strings.HasPrefix(operation, "testing_library_query:"):
		return throwingAcceptanceQuery(operation) && grammar.allowRootedObservation(expression, awaited)
	case strings.HasPrefix(operation, "expect_matcher:"):
		return grammar.allowMatcher(expression, operation)
	case strings.HasPrefix(operation, "fire_event:"):
		return grammar.allowFireEvent(expression, operation)
	case operation == "harness_call:waitFor":
		return awaited && grammar.allowWaitFor(expression)
	default:
		return false
	}
}

func (grammar acceptanceObserverGrammar) allowMatcher(call *treesitter.Node, operation string) bool {
	if !acceptanceRegisteredMatcher(strings.TrimPrefix(operation, "expect_matcher:")) {
		return false
	}
	expectCall, _, trusted := acceptanceMatcherChain(call, grammar.source)
	if !trusted || expectCall == nil {
		return false
	}
	expectArguments := expectCall.ChildByFieldName("arguments")
	if expectArguments == nil || expectArguments.NamedChildCount() != 1 ||
		!grammar.allowRootedObservation(expectArguments.NamedChild(0), false) {
		return false
	}
	if !grammar.allowClosedArguments(call.ChildByFieldName("arguments"), operation) {
		return false
	}
	grammar.trustedCalls[call.Id()] = struct{}{}
	grammar.trustedCalls[expectCall.Id()] = struct{}{}
	return true
}

func (grammar acceptanceObserverGrammar) allowFireEvent(call *treesitter.Node, operation string) bool {
	if !acceptanceRegisteredEvent(strings.TrimPrefix(operation, "fire_event:")) {
		return false
	}
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() < 1 || arguments.NamedChildCount() > 2 ||
		!grammar.allowRootedObservation(arguments.NamedChild(0), false) {
		return false
	}
	if arguments.NamedChildCount() == 2 && !grammar.allowClosedValue(arguments.NamedChild(1), operation) {
		return false
	}
	grammar.trustedCalls[call.Id()] = struct{}{}
	return true
}

func (grammar acceptanceObserverGrammar) allowWaitFor(call *treesitter.Node) bool {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return false
	}
	callback := arguments.NamedChild(0)
	if callback == nil || callback.Kind() != "arrow_function" && callback.Kind() != "function_expression" {
		return false
	}
	parameters := callback.ChildByFieldName("parameters")
	if parameters != nil && parameters.NamedChildCount() != 0 {
		return false
	}
	body := callback.ChildByFieldName("body")
	if body == nil {
		return false
	}
	if body.Kind() == "statement_block" {
		executable := 0
		for index := uint(0); index < body.NamedChildCount(); index++ {
			statement := body.NamedChild(index)
			if statement != nil && statement.Kind() != "comment" {
				executable++
				if !grammar.allowStatement(statement) {
					return false
				}
			}
		}
		if executable == 0 {
			return false
		}
	} else {
		if body.Kind() != "call_expression" || !grammar.allowExpressionCall(body) {
			return false
		}
	}
	grammar.trustedCalls[call.Id()] = struct{}{}
	return true
}

func (grammar acceptanceObserverGrammar) allowExpressionCall(call *treesitter.Node) bool {
	statementKind := acceptanceCallOperation(call, grammar.source)
	switch {
	case strings.HasPrefix(statementKind, "expect_matcher:"):
		return grammar.allowMatcher(call, statementKind)
	case strings.HasPrefix(statementKind, "fire_event:"):
		return grammar.allowFireEvent(call, statementKind)
	case strings.HasPrefix(statementKind, "testing_library_query:"):
		return throwingAcceptanceQuery(statementKind) && grammar.allowRootedObservation(call, false)
	default:
		return false
	}
}

func (grammar acceptanceObserverGrammar) allowRootedObservation(
	node *treesitter.Node,
	awaited bool,
) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "await_expression":
		return node.NamedChildCount() == 1 && grammar.allowRootedObservation(node.NamedChild(0), true)
	case "parenthesized_expression":
		return node.NamedChildCount() == 1 && grammar.allowRootedObservation(node.NamedChild(0), awaited)
	case "member_expression":
		property := node.ChildByFieldName("property")
		return property != nil && acceptancePublicObservationProperty(property.Utf8Text(grammar.source)) &&
			grammar.allowRootedObservation(node.ChildByFieldName("object"), awaited)
	case "subscript_expression":
		index := node.ChildByFieldName("index")
		return staticAcceptanceValue(index, grammar.source) &&
			grammar.allowRootedObservation(node.ChildByFieldName("object"), awaited)
	case "call_expression":
		operation := acceptanceCallOperation(node, grammar.source)
		arguments := node.ChildByFieldName("arguments")
		if !strings.HasPrefix(operation, "testing_library_query:") ||
			asyncAcceptanceQuery(operation) && !awaited ||
			arguments == nil || arguments.NamedChildCount() == 0 ||
			!grammar.allowClosedArguments(arguments, operation) {
			return false
		}
		grammar.trustedCalls[node.Id()] = struct{}{}
		return true
	default:
		return false
	}
}

func asyncAcceptanceQuery(operation string) bool {
	name := strings.TrimPrefix(operation, "testing_library_query:")
	return strings.HasPrefix(name, "findBy") || strings.HasPrefix(name, "findAllBy")
}

func (grammar acceptanceObserverGrammar) allowClosedArguments(
	arguments *treesitter.Node,
	operation string,
) bool {
	if arguments == nil {
		return false
	}
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		if !grammar.allowClosedValue(arguments.NamedChild(index), operation) {
			return false
		}
	}
	return true
}

func (grammar acceptanceObserverGrammar) allowClosedValue(node *treesitter.Node, operation string) bool {
	if staticAcceptanceValue(node, grammar.source) {
		return true
	}
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "array":
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if !grammar.allowClosedValue(node.NamedChild(index), operation) {
				return false
			}
		}
		return true
	case "object":
		for index := uint(0); index < node.NamedChildCount(); index++ {
			pair := node.NamedChild(index)
			if pair == nil || pair.Kind() != "pair" || !grammar.allowClosedPair(pair, operation) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (grammar acceptanceObserverGrammar) allowClosedPair(pair *treesitter.Node, operation string) bool {
	key := pair.ChildByFieldName("key")
	if key == nil {
		return false
	}
	name := key.Utf8Text(grammar.source)
	if key.Kind() == "string" {
		name = decodeAcceptanceString(name)
	}
	return acceptanceSemanticFieldOperation(operation, name) != "" &&
		grammar.allowClosedValue(pair.ChildByFieldName("value"), operation)
}

func staticAcceptanceValue(node *treesitter.Node, source []byte) bool {
	_, ok := acceptanceLiteral(node, source)
	return ok
}

func throwingAcceptanceQuery(operation string) bool {
	name := strings.TrimPrefix(operation, "testing_library_query:")
	return strings.HasPrefix(name, "getBy") || strings.HasPrefix(name, "getAllBy") ||
		strings.HasPrefix(name, "findBy") || strings.HasPrefix(name, "findAllBy")
}
