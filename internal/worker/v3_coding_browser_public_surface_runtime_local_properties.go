package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserRuntimeLocalProperties map[uintptr]map[string]struct{}

func collectDirectCodingBrowserRuntimeLocalProperties(
	root *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) directCodingBrowserRuntimeLocalProperties {
	properties := make(directCodingBrowserRuntimeLocalProperties)
	var walk func(*treesitter.Node)
	walk = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		if node.Kind() == "variable_declarator" {
			declaration := directCodingBrowserRuntimeDeclaration(node)
			name := node.ChildByFieldName("name")
			value := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("value"))
			if directCodingBrowserDeclarationIsConst(declaration) &&
				name != nil && name.Kind() == "identifier" && value != nil &&
				value.Kind() == "object" {
				owned := directCodingBrowserRuntimeObjectProperties(value, source)
				if len(owned) != 0 {
					properties[name.Id()] = owned
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			walk(node.NamedChild(index))
		}
	}
	walk(root)
	invalidateDirectCodingBrowserMutatedLocalProperties(root, source, bindings, properties)
	return properties
}

func invalidateDirectCodingBrowserMutatedLocalProperties(
	root *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	properties directCodingBrowserRuntimeLocalProperties,
) {
	var walk func(*treesitter.Node)
	walk = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "assignment_expression", "augmented_assignment_expression":
			invalidateDirectCodingBrowserLocalPropertyTarget(
				node.ChildByFieldName("left"), source, bindings, properties,
			)
		case "update_expression":
			target := node.ChildByFieldName("argument")
			if target == nil && node.NamedChildCount() == 1 {
				target = node.NamedChild(0)
			}
			invalidateDirectCodingBrowserLocalPropertyTarget(
				target, source, bindings, properties,
			)
		case "unary_expression":
			if strings.HasPrefix(strings.TrimSpace(directCodingBrowserRuntimeNodeText(source, node)), "delete ") {
				invalidateDirectCodingBrowserLocalPropertyTarget(
					node.ChildByFieldName("argument"), source, bindings, properties,
				)
			}
		case "call_expression":
			callee := directCodingBrowserRuntimeNodeText(source, node.ChildByFieldName("function"))
			arguments := node.ChildByFieldName("arguments")
			if callee == "Object.assign" && arguments != nil && arguments.NamedChildCount() > 0 {
				invalidateDirectCodingBrowserLocalPropertyReceiver(
					arguments.NamedChild(0), source, bindings, properties,
				)
			}
			if arguments != nil {
				for index := uint(0); index < arguments.NamedChildCount(); index++ {
					invalidateDirectCodingBrowserLocalPropertyReceiver(
						arguments.NamedChild(index), source, bindings, properties,
					)
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			walk(node.NamedChild(index))
		}
	}
	walk(root)
}

func invalidateDirectCodingBrowserLocalPropertyTarget(
	target *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	properties directCodingBrowserRuntimeLocalProperties,
) {
	target = directCodingBrowserUnwrapRuntimeExpression(target)
	name, resolved, property := directCodingBrowserRuntimeProperty(source, target)
	if !property || !resolved {
		return
	}
	receiver := directCodingBrowserUnwrapRuntimeExpression(target.ChildByFieldName("object"))
	if receiver == nil || receiver.Kind() != "identifier" {
		return
	}
	binding, found := bindings.resolve(
		directCodingBrowserRuntimeNodeText(source, receiver), receiver,
	)
	if !found {
		return
	}
	delete(properties[binding.declarationID], name)
}

func invalidateDirectCodingBrowserLocalPropertyReceiver(
	receiver *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	properties directCodingBrowserRuntimeLocalProperties,
) {
	receiver = directCodingBrowserUnwrapRuntimeExpression(receiver)
	if receiver == nil || receiver.Kind() != "identifier" {
		return
	}
	binding, found := bindings.resolve(
		directCodingBrowserRuntimeNodeText(source, receiver), receiver,
	)
	if found {
		delete(properties, binding.declarationID)
	}
}

func (properties directCodingBrowserRuntimeLocalProperties) receiverOwns(
	receiver *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	receiver = directCodingBrowserUnwrapRuntimeExpression(receiver)
	if receiver == nil {
		return false
	}
	if receiver.Kind() == "object" {
		_, owned := directCodingBrowserRuntimeObjectProperties(receiver, source)[name]
		return owned
	}
	if receiver.Kind() != "identifier" {
		return false
	}
	binding, resolved := bindings.resolve(
		directCodingBrowserRuntimeNodeText(source, receiver), receiver,
	)
	if !resolved {
		return false
	}
	_, owned := properties[binding.declarationID][name]
	return owned
}

func (properties directCodingBrowserRuntimeLocalProperties) patternOwns(
	pattern *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	for current := pattern; current != nil; current = current.Parent() {
		if current.Kind() == "variable_declarator" {
			return properties.receiverOwns(
				current.ChildByFieldName("value"), name, source, bindings,
			)
		}
		if javaScriptFunctionScopeKind(current.Kind()) {
			return false
		}
	}
	return false
}

func directCodingBrowserRuntimeObjectProperties(
	object *treesitter.Node,
	source []byte,
) map[string]struct{} {
	properties := make(map[string]struct{})
	if object == nil || object.Kind() != "object" {
		return properties
	}
	for index := uint(0); index < object.NamedChildCount(); index++ {
		property := object.NamedChild(index)
		if property == nil || property.Kind() == "comment" {
			continue
		}
		name, ok := directCodingBrowserRuntimeObjectPropertyName(property, source)
		if !ok {
			return map[string]struct{}{}
		}
		properties[name] = struct{}{}
	}
	return properties
}

func directCodingBrowserRuntimeObjectPropertyName(
	node *treesitter.Node,
	source []byte,
) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "pair":
		return directCodingBrowserRuntimePatternProperty(source, node.ChildByFieldName("key"))
	case "method_definition":
		name := node.ChildByFieldName("name")
		if name == nil && node.NamedChildCount() > 0 {
			name = node.NamedChild(0)
		}
		return directCodingBrowserRuntimePatternProperty(source, name)
	case "shorthand_property_identifier":
		return directCodingBrowserRuntimeNodeText(source, node), true
	default:
		return "", false
	}
}
