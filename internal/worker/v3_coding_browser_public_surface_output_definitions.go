package worker

import (
	"strconv"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (flow *directCodingBrowserOutputDataflow) collectDeclarations(node *treesitter.Node) {
	if node == nil {
		return
	}
	if javaScriptFunctionScopeKind(node.Kind()) {
		if node.Kind() == "function_declaration" {
			flow.add(
				node.ChildByFieldName("name"), directCodingBrowserOutputFunction,
				node, directCodingBrowserRuntimeParentScope(node), node.StartByte(),
			)
		}
		flow.collectFunction(node, false)
		return
	}
	if node.Kind() == "variable_declarator" {
		flow.collectVariable(node)
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		flow.collectDeclarations(node.NamedChild(index))
	}
}

func (flow *directCodingBrowserOutputDataflow) collectVariable(
	declarator *treesitter.Node,
) {
	pattern := declarator.ChildByFieldName("name")
	value := declarator.ChildByFieldName("value")
	declaration := directCodingBrowserRuntimeDeclaration(declarator)
	scope := directCodingBrowserRuntimeLexicalScope(declaration)
	if declaration != nil && declaration.Kind() == "variable_declaration" {
		scope = directCodingBrowserRuntimeFunctionScope(declaration)
	}
	if state, setter, ok := directCodingBrowserUseStatePattern(pattern, value, flow.source); ok {
		stateDefinition := flow.add(
			state, directCodingBrowserOutputLocalState, nil, scope, declarator.EndByte(),
		)
		setterDefinition := flow.add(
			setter, directCodingBrowserOutputSetter, nil, scope, declarator.EndByte(),
		)
		if stateDefinition != nil && setterDefinition != nil {
			stateDefinition.setterID = setterDefinition.id
		}
		return
	}
	flow.collectAliasPattern(pattern, value, scope, declarator.EndByte())
}

func (flow *directCodingBrowserOutputDataflow) collectAliasPattern(
	pattern *treesitter.Node,
	value *treesitter.Node,
	scope *treesitter.Node,
	available uint,
) {
	if pattern == nil {
		return
	}
	switch pattern.Kind() {
	case "identifier", "shorthand_property_identifier_pattern":
		flow.add(pattern, directCodingBrowserOutputAlias, value, scope, available)
	case "required_parameter", "optional_parameter":
		child := pattern.ChildByFieldName("pattern")
		if child == nil && pattern.NamedChildCount() > 0 {
			child = pattern.NamedChild(0)
		}
		flow.collectAliasPattern(child, value, scope, available)
	case "assignment_pattern", "object_assignment_pattern":
		left := pattern.ChildByFieldName("left")
		if left == nil {
			left = pattern.ChildByFieldName("pattern")
		}
		if value == nil {
			value = pattern.ChildByFieldName("right")
		}
		flow.collectAliasPattern(left, value, scope, available)
	case "object_pattern":
		flow.collectObjectAliasPattern(pattern, value, scope, available)
	case "array_pattern":
		flow.collectArrayAliasPattern(pattern, value, scope, available)
	case "rest_pattern":
		argument := pattern.ChildByFieldName("argument")
		if argument == nil && pattern.NamedChildCount() > 0 {
			argument = pattern.NamedChild(0)
		}
		flow.collectAliasPattern(argument, nil, scope, available)
	default:
		for _, binding := range directCodingBrowserOutputPatternBindings(pattern) {
			flow.add(binding, directCodingBrowserOutputAlias, nil, scope, available)
		}
	}
}

func (flow *directCodingBrowserOutputDataflow) collectObjectAliasPattern(
	pattern *treesitter.Node,
	value *treesitter.Node,
	scope *treesitter.Node,
	available uint,
) {
	for index := uint(0); index < pattern.NamedChildCount(); index++ {
		property := pattern.NamedChild(index)
		if property == nil || property.Kind() == "comment" {
			continue
		}
		if property.Kind() == "rest_pattern" {
			flow.collectAliasPattern(property, nil, scope, available)
			continue
		}
		binding := property
		keyNode := property
		if property.Kind() == "pair_pattern" {
			keyNode = property.ChildByFieldName("key")
			binding = property.ChildByFieldName("value")
		} else if property.Kind() == "object_assignment_pattern" {
			keyNode = property.ChildByFieldName("left")
			binding = property
		}
		key, resolved := directCodingBrowserRuntimePatternProperty(flow.source, keyNode)
		if !resolved {
			flow.collectAliasPattern(binding, nil, scope, available)
			continue
		}
		selected, structural := flow.projectOutputProperty(
			value, key, make(map[uintptr]struct{}),
		)
		if !structural {
			selected = value
		}
		flow.collectAliasPattern(binding, selected, scope, available)
	}
}

func (flow *directCodingBrowserOutputDataflow) collectArrayAliasPattern(
	pattern *treesitter.Node,
	value *treesitter.Node,
	scope *treesitter.Node,
	available uint,
) {
	position := uint64(0)
	for index := uint(0); index < pattern.ChildCount(); index++ {
		binding := pattern.Child(index)
		if binding == nil {
			continue
		}
		switch binding.Kind() {
		case "[", "]", "comment":
			continue
		case ",":
			position++
		case "rest_pattern":
			flow.collectAliasPattern(binding, nil, scope, available)
		default:
			selected, structural := flow.projectOutputProperty(
				value, strconv.FormatUint(position, 10), make(map[uintptr]struct{}),
			)
			if !structural {
				selected = value
			}
			flow.collectAliasPattern(binding, selected, scope, available)
		}
	}
}

func (flow *directCodingBrowserOutputDataflow) collectParameterPattern(
	pattern *treesitter.Node,
	scope *treesitter.Node,
	root bool,
) {
	for _, binding := range directCodingBrowserOutputPatternBindings(pattern) {
		kind := directCodingBrowserOutputOpaque
		if root {
			name := directCodingBrowserRuntimeNodeText(flow.source, binding)
			if name == "state" || name == "capabilities" {
				kind = directCodingBrowserOutputRootState
			}
		}
		flow.add(binding, kind, nil, scope, scope.StartByte())
	}
}

func directCodingBrowserOutputPatternBindings(pattern *treesitter.Node) []*treesitter.Node {
	bindings := make([]*treesitter.Node, 0, 2)
	var collect func(*treesitter.Node)
	collect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "identifier", "shorthand_property_identifier_pattern":
			bindings = append(bindings, node)
			return
		case "pair_pattern":
			collect(node.ChildByFieldName("value"))
			return
		case "required_parameter", "optional_parameter":
			value := node.ChildByFieldName("pattern")
			if value == nil && node.NamedChildCount() > 0 {
				value = node.NamedChild(0)
			}
			collect(value)
			return
		case "assignment_pattern", "object_assignment_pattern":
			collect(node.ChildByFieldName("left"))
			return
		case "rest_pattern":
			collect(node.ChildByFieldName("argument"))
			return
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			collect(node.NamedChild(index))
		}
	}
	collect(pattern)
	return bindings
}

func directCodingBrowserUseStatePattern(
	pattern *treesitter.Node,
	value *treesitter.Node,
	source []byte,
) (*treesitter.Node, *treesitter.Node, bool) {
	if pattern == nil || pattern.Kind() != "array_pattern" || value == nil ||
		value.Kind() != "call_expression" {
		return nil, nil, false
	}
	callee := value.ChildByFieldName("function")
	bindings := directCodingBrowserOutputPatternBindings(pattern)
	if callee == nil || directCodingBrowserRuntimeNodeText(source, callee) != "useState" ||
		len(bindings) != 2 || bindings[0].Kind() != "identifier" ||
		bindings[1].Kind() != "identifier" {
		return nil, nil, false
	}
	return bindings[0], bindings[1], true
}
