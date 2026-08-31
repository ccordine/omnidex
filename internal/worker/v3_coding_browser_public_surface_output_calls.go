package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

type directCodingBrowserOutputCallFrame struct {
	arguments map[uintptr]*treesitter.Node
	parent    *directCodingBrowserOutputCallFrame
}

type directCodingBrowserOutputCallProjection struct {
	callable  *treesitter.Node
	value     *treesitter.Node
	arguments map[uintptr]*treesitter.Node
}

func (flow directCodingBrowserOutputDataflow) projectLocalOutputCallMember(
	node *treesitter.Node,
) (*treesitter.Node, directCodingBrowserOutputCallProjection, bool) {
	if node == nil || (node.Kind() != "member_expression" &&
		node.Kind() != "subscript_expression") {
		return nil, directCodingBrowserOutputCallProjection{}, false
	}
	receiver := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("object"))
	if receiver == nil || receiver.Kind() != "call_expression" {
		return nil, directCodingBrowserOutputCallProjection{}, false
	}
	projection, projected := flow.projectLocalOutputCall(receiver)
	if !projected {
		return nil, directCodingBrowserOutputCallProjection{}, false
	}
	key, resolved := flow.outputPropertyName(node)
	if !resolved {
		return nil, projection, true
	}
	selected, structural := flow.projectOutputProperty(
		projection.value, key, make(map[uintptr]struct{}),
	)
	if !structural {
		return nil, projection, true
	}
	return selected, projection, true
}

func (frame *directCodingBrowserOutputCallFrame) argument(
	definitionID uintptr,
) (*treesitter.Node, *directCodingBrowserOutputCallFrame, bool) {
	for current := frame; current != nil; current = current.parent {
		if argument, exists := current.arguments[definitionID]; exists {
			return argument, current.parent, true
		}
	}
	return nil, frame, false
}

func (flow directCodingBrowserOutputDataflow) projectLocalOutputCall(
	call *treesitter.Node,
) (directCodingBrowserOutputCallProjection, bool) {
	if call == nil || call.Kind() != "call_expression" {
		return directCodingBrowserOutputCallProjection{}, false
	}
	callee := directCodingBrowserUnwrapRuntimeExpression(call.ChildByFieldName("function"))
	arguments, validArguments := directCodingBrowserOutputCallArguments(call)
	if !validArguments {
		return directCodingBrowserOutputCallProjection{}, false
	}
	if callee != nil && callee.Kind() == "identifier" &&
		flow.text(callee) == "useMemo" && flow.resolve("useMemo", callee) == nil {
		if len(arguments) == 0 {
			return directCodingBrowserOutputCallProjection{}, false
		}
		callable := flow.resolveOutputCallable(arguments[0], make(map[uintptr]struct{}))
		return flow.projectOutputFunctionInvocation(callable, nil)
	}
	callable := flow.resolveOutputCallable(callee, make(map[uintptr]struct{}))
	return flow.projectOutputFunctionInvocation(callable, arguments)
}

func (flow directCodingBrowserOutputDataflow) resolveOutputCallable(
	node *treesitter.Node,
	visiting map[uintptr]struct{},
) *treesitter.Node {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "arrow_function", "function_expression", "function_declaration", "method_definition":
		return node
	case "identifier", "shorthand_property_identifier":
		definition := flow.resolve(flow.text(node), node)
		if definition == nil || (definition.kind != directCodingBrowserOutputAlias &&
			definition.kind != directCodingBrowserOutputFunction) {
			return nil
		}
		if _, cycle := visiting[definition.id]; cycle {
			return nil
		}
		visiting[definition.id] = struct{}{}
		callable := flow.resolveOutputCallable(definition.value, visiting)
		delete(visiting, definition.id)
		return callable
	case "call_expression":
		callee := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("function"))
		if callee == nil || callee.Kind() != "identifier" || flow.text(callee) != "useCallback" ||
			flow.resolve("useCallback", callee) != nil {
			return nil
		}
		arguments, valid := directCodingBrowserOutputCallArguments(node)
		if !valid || len(arguments) == 0 {
			return nil
		}
		return flow.resolveOutputCallable(arguments[0], visiting)
	case "member_expression", "subscript_expression":
		key, resolved := flow.outputPropertyName(node)
		if !resolved {
			return nil
		}
		selected, structural := flow.projectOutputProperty(
			node.ChildByFieldName("object"), key, make(map[uintptr]struct{}),
		)
		if !structural || selected == nil {
			return nil
		}
		return flow.resolveOutputCallable(selected, visiting)
	default:
		return nil
	}
}

func (flow directCodingBrowserOutputDataflow) projectOutputFunctionInvocation(
	callable *treesitter.Node,
	arguments []*treesitter.Node,
) (directCodingBrowserOutputCallProjection, bool) {
	if callable == nil {
		return directCodingBrowserOutputCallProjection{}, false
	}
	value, resolved := directCodingBrowserOutputFunctionValue(callable)
	if !resolved {
		return directCodingBrowserOutputCallProjection{}, false
	}
	parameters, resolved := directCodingBrowserOutputFunctionParameters(callable)
	if !resolved {
		return directCodingBrowserOutputCallProjection{}, false
	}
	bound := make(map[uintptr]*treesitter.Node, len(parameters))
	for index, parameter := range parameters {
		if index < len(arguments) {
			bound[parameter.Id()] = arguments[index]
		}
	}
	return directCodingBrowserOutputCallProjection{
		callable: callable, value: value, arguments: bound,
	}, true
}

func directCodingBrowserOutputFunctionValue(
	function *treesitter.Node,
) (*treesitter.Node, bool) {
	if function == nil {
		return nil, false
	}
	body := function.ChildByFieldName("body")
	if body == nil {
		return nil, false
	}
	if function.Kind() == "arrow_function" && body.Kind() != "statement_block" {
		return body, true
	}
	if body.Kind() != "statement_block" {
		return nil, false
	}
	returns, throws := directCodingBrowserRenderControlFlow(body)
	if len(returns) != 1 || len(throws) != 0 || returns[0].Parent() == nil ||
		returns[0].Parent().Id() != body.Id() || returns[0].NamedChildCount() != 1 {
		return nil, false
	}
	return returns[0].NamedChild(0), true
}

func directCodingBrowserOutputFunctionParameters(
	function *treesitter.Node,
) ([]*treesitter.Node, bool) {
	parameters := function.ChildByFieldName("parameters")
	if parameters == nil {
		parameters = function.ChildByFieldName("parameter")
	}
	if parameters == nil {
		return nil, true
	}
	candidates := make([]*treesitter.Node, 0, parameters.NamedChildCount())
	if parameters.Kind() == "formal_parameters" {
		for index := uint(0); index < parameters.NamedChildCount(); index++ {
			candidate := parameters.NamedChild(index)
			if candidate != nil && candidate.Kind() != "comment" {
				candidates = append(candidates, candidate)
			}
		}
	} else {
		candidates = append(candidates, parameters)
	}
	result := make([]*treesitter.Node, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Kind() == "required_parameter" {
			candidate = candidate.ChildByFieldName("pattern")
			if candidate == nil {
				return nil, false
			}
		}
		if candidate.Kind() != "identifier" {
			return nil, false
		}
		result = append(result, candidate)
	}
	return result, true
}

func directCodingBrowserOutputCallArguments(
	call *treesitter.Node,
) ([]*treesitter.Node, bool) {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil {
		return nil, false
	}
	result := make([]*treesitter.Node, 0, arguments.NamedChildCount())
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		argument := arguments.NamedChild(index)
		if argument == nil || argument.Kind() == "comment" {
			continue
		}
		if argument.Kind() == "spread_element" {
			return nil, false
		}
		result = append(result, argument)
	}
	return result, true
}
