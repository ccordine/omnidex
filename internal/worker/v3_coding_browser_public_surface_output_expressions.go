package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func (extractor *directCodingBrowserPublicSurfaceExtractor) outputExpressionIsRuntimeDerived(
	expression *treesitter.Node,
) bool {
	return extractor.outputFlow.derived(
		expression, make(map[uintptr]struct{}), nil,
	)
}

func (flow directCodingBrowserOutputDataflow) derived(
	node *treesitter.Node,
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
		derived := flow.derived(
			selected, visiting,
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
			derived := flow.derived(
				projection.value, visiting,
				&directCodingBrowserOutputCallFrame{
					arguments: projection.arguments, parent: frame,
				},
			)
			delete(visiting, projection.callable.Id())
			return derived
		}
	}
	if children, projected := flow.outputValueChildren(node); projected {
		for _, child := range children {
			if flow.derived(child, visiting, frame) {
				return true
			}
		}
		return false
	}
	switch node.Kind() {
	case "identifier", "shorthand_property_identifier":
		definition := flow.resolve(flow.text(node), node)
		if definition == nil {
			return false
		}
		if frame != nil {
			if argument, parent, bound := frame.argument(definition.id); bound {
				return flow.derived(argument, visiting, parent)
			}
		}
		switch definition.kind {
		case directCodingBrowserOutputRootState:
			return true
		case directCodingBrowserOutputLocalState:
			_, called := flow.calledSetters[definition.setterID]
			return called
		case directCodingBrowserOutputAlias:
			if _, cycle := visiting[definition.id]; cycle {
				return false
			}
			visiting[definition.id] = struct{}{}
			derived := flow.derived(definition.value, visiting, frame)
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
		if flow.derived(node.NamedChild(index), visiting, frame) {
			return true
		}
	}
	return false
}

func (flow directCodingBrowserOutputDataflow) resolve(
	name string,
	reference *treesitter.Node,
) *directCodingBrowserOutputDefinition {
	var match *directCodingBrowserOutputDefinition
	matchSpan := ^uint(0)
	for _, candidate := range flow.definitions[name] {
		if candidate.nodeID == reference.Id() ||
			reference.StartByte() < candidate.available ||
			reference.StartByte() < candidate.scopeStart ||
			reference.EndByte() > candidate.scopeEnd {
			continue
		}
		span := candidate.scopeEnd - candidate.scopeStart
		if match != nil && span == matchSpan {
			return nil
		}
		if match == nil || span < matchSpan {
			match = candidate
			matchSpan = span
		}
	}
	return match
}

func (flow directCodingBrowserOutputDataflow) text(node *treesitter.Node) string {
	return directCodingBrowserRuntimeNodeText(flow.source, node)
}
