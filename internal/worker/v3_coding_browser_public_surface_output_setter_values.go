package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func (flow directCodingBrowserOutputDataflow) setterArgumentDerived(
	argument *treesitter.Node,
	handler *treesitter.Node,
) bool {
	updater := directCodingBrowserUnwrapRuntimeExpression(argument)
	updaterBindings := directCodingBrowserEventBindings(nil)
	if updater != nil && (updater.Kind() == "arrow_function" ||
		updater.Kind() == "function_expression") {
		parameter, err := directCodingBrowserEventParameter(updater)
		if err != nil || parameter == nil {
			return false
		}
		updaterBindings = directCodingBrowserEventBindings{{
			name:  directCodingBrowserRuntimeNodeText(flow.source, parameter),
			start: updater.StartByte(), end: updater.EndByte(), declarationID: parameter.Id(),
		}}
	}
	return flow.setterValueDerived(
		argument, handler, updaterBindings, make(map[uintptr]struct{}), nil,
	)
}

func (flow directCodingBrowserOutputDataflow) setterValueDerived(
	node *treesitter.Node,
	handler *treesitter.Node,
	updaterBindings directCodingBrowserEventBindings,
	visiting map[uintptr]struct{},
	frame *directCodingBrowserOutputCallFrame,
) bool {
	if node == nil {
		return false
	}
	if selected, projection, projected := flow.projectLocalOutputCallMember(node); projected {
		if selected == nil {
			return false
		}
		if _, cycle := visiting[projection.callable.Id()]; cycle {
			return false
		}
		visiting[projection.callable.Id()] = struct{}{}
		derived := flow.setterValueDerived(
			selected, handler, updaterBindings, visiting,
			&directCodingBrowserOutputCallFrame{
				arguments: projection.arguments, parent: frame,
			},
		)
		delete(visiting, projection.callable.Id())
		return derived
	}
	if node.Kind() == "call_expression" {
		if projection, projected := flow.projectLocalOutputCall(node); projected {
			if _, cycle := visiting[projection.callable.Id()]; cycle {
				return false
			}
			visiting[projection.callable.Id()] = struct{}{}
			derived := flow.setterValueDerived(
				projection.value, handler, updaterBindings, visiting,
				&directCodingBrowserOutputCallFrame{
					arguments: projection.arguments, parent: frame,
				},
			)
			delete(visiting, projection.callable.Id())
			return derived
		}
	}
	if name, resolved, property := directCodingBrowserRuntimeProperty(flow.source, node); property && resolved && (name == "value" || name == "checked") &&
		directCodingBrowserExpressionIsEventTarget(
			node.ChildByFieldName("object"), flow.source, flow.eventBindings,
		) {
		return true
	}
	if children, projected := flow.outputValueChildren(node); projected {
		for _, child := range children {
			if flow.setterValueDerived(
				child, handler, updaterBindings, visiting, frame,
			) {
				return true
			}
		}
		return false
	}
	switch node.Kind() {
	case "identifier", "shorthand_property_identifier":
		if updaterBindings.reference(node, flow.source) {
			return true
		}
		definition := flow.resolve(flow.text(node), node)
		if definition == nil {
			return false
		}
		if frame != nil {
			if argument, parent, bound := frame.argument(definition.id); bound {
				return flow.setterValueDerived(
					argument, handler, updaterBindings, visiting, parent,
				)
			}
		}
		switch definition.kind {
		case directCodingBrowserOutputRootState:
			return true
		case directCodingBrowserOutputLocalState:
			_, resolved := flow.calledSetters[definition.setterID]
			return resolved
		case directCodingBrowserOutputAlias:
			if _, cycle := visiting[definition.id]; cycle {
				return false
			}
			visiting[definition.id] = struct{}{}
			derived := flow.setterValueDerived(
				definition.value, handler, updaterBindings, visiting, frame,
			)
			delete(visiting, definition.id)
			return derived
		default:
			return false
		}
	case "property_identifier", "private_property_identifier", "type_identifier",
		"predefined_type", "comment", "number", "string", "regex", "true",
		"false", "null", "undefined", "this":
		return false
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if flow.setterValueDerived(
			node.NamedChild(index), handler, updaterBindings, visiting, frame,
		) {
			return true
		}
	}
	return false
}
