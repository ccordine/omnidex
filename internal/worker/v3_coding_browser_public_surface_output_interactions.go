package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

type directCodingBrowserSetterCall struct {
	argument *treesitter.Node
	handler  *treesitter.Node
}

func (flow *directCodingBrowserOutputDataflow) collectInteractionSetters(
	render *treesitter.Node,
) {
	var inspect func(*treesitter.Node)
	inspect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		if node.Kind() == "jsx_attribute" && node.NamedChildCount() > 1 {
			name := node.NamedChild(0)
			if name != nil && directCodingBrowserAllowedEventAttribute(
				directCodingBrowserRuntimeNodeText(flow.source, name),
			) {
				if handler := flow.resolveOutputEventHandler(node.NamedChild(1)); handler != nil {
					flow.collectDirectSetterCalls(handler, handler)
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			inspect(node.NamedChild(index))
		}
	}
	inspect(render)
}

func (flow directCodingBrowserOutputDataflow) resolveOutputEventHandler(
	node *treesitter.Node,
) *treesitter.Node {
	expression := directCodingBrowserUnwrapRuntimeExpression(node)
	if expression != nil && expression.Kind() == "jsx_expression" &&
		expression.NamedChildCount() == 1 {
		expression = directCodingBrowserUnwrapRuntimeExpression(expression.NamedChild(0))
	}
	if function := directCodingBrowserBoundEventFunction(expression, flow.source); function != nil {
		return function
	}
	if expression == nil || expression.Kind() != "identifier" {
		return nil
	}
	definition := flow.resolve(flow.text(expression), expression)
	if definition == nil || (definition.kind != directCodingBrowserOutputAlias &&
		definition.kind != directCodingBrowserOutputFunction) {
		return nil
	}
	return directCodingBrowserBoundEventFunction(definition.value, flow.source)
}

func (flow *directCodingBrowserOutputDataflow) collectDirectSetterCalls(
	node *treesitter.Node,
	handler *treesitter.Node,
) {
	if node == nil {
		return
	}
	if node != handler && javaScriptFunctionScopeKind(node.Kind()) {
		return
	}
	if node.Kind() == "call_expression" {
		callee := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("function"))
		arguments := node.ChildByFieldName("arguments")
		if callee != nil && callee.Kind() == "identifier" && arguments != nil &&
			arguments.NamedChildCount() == 1 &&
			directCodingBrowserSetterCallPotentiallyReachable(node, handler, flow.source) {
			definition := flow.resolve(flow.text(callee), callee)
			if definition != nil && definition.kind == directCodingBrowserOutputSetter {
				flow.setterCalls[definition.id] = append(
					flow.setterCalls[definition.id],
					directCodingBrowserSetterCall{
						argument: arguments.NamedChild(0), handler: handler,
					},
				)
			}
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		flow.collectDirectSetterCalls(node.NamedChild(index), handler)
	}
}

func (flow *directCodingBrowserOutputDataflow) resolveInteractionSetters() {
	for changed := true; changed; {
		changed = false
		for setterID, calls := range flow.setterCalls {
			if _, resolved := flow.calledSetters[setterID]; resolved {
				continue
			}
			for _, call := range calls {
				if flow.setterArgumentDerived(call.argument, call.handler) {
					flow.calledSetters[setterID] = struct{}{}
					changed = true
					break
				}
			}
		}
	}
}
