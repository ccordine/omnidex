package worker

import (
	"strconv"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (flow directCodingBrowserOutputDataflow) projectMemberValue(
	node *treesitter.Node,
) (*treesitter.Node, bool) {
	if node == nil || (node.Kind() != "member_expression" &&
		node.Kind() != "subscript_expression") {
		return nil, false
	}
	object := node.ChildByFieldName("object")
	key, resolved := flow.outputPropertyName(node)
	if !resolved {
		return nil, true
	}
	selected, structural := flow.projectOutputProperty(
		object, key, make(map[uintptr]struct{}),
	)
	if structural {
		return selected, true
	}
	// A non-local receiver such as code-owned state is itself the value
	// authority for one statically selected property.
	return object, true
}

func (flow directCodingBrowserOutputDataflow) outputPropertyName(
	node *treesitter.Node,
) (string, bool) {
	if node == nil {
		return "", false
	}
	if node.Kind() == "member_expression" {
		property := node.ChildByFieldName("property")
		if property == nil {
			return "", false
		}
		return directCodingBrowserRuntimeNodeText(flow.source, property), true
	}
	return flow.staticOutputPropertyName(
		node.ChildByFieldName("index"), make(map[uintptr]struct{}),
	)
}

func (flow directCodingBrowserOutputDataflow) staticOutputPropertyName(
	node *treesitter.Node,
	visiting map[uintptr]struct{},
) (string, bool) {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return "", false
	}
	if value, resolved := javaScriptStaticPropertyName(flow.source, node); resolved {
		return value, true
	}
	if node.Kind() == "number" {
		raw := strings.ReplaceAll(flow.text(node), "_", "")
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", false
		}
		return strconv.FormatFloat(value, 'f', -1, 64), true
	}
	if node.Kind() != "identifier" {
		return "", false
	}
	definition := flow.resolve(flow.text(node), node)
	if definition == nil || definition.kind != directCodingBrowserOutputAlias {
		return "", false
	}
	if _, cycle := visiting[definition.id]; cycle {
		return "", false
	}
	visiting[definition.id] = struct{}{}
	value, resolved := flow.staticOutputPropertyName(definition.value, visiting)
	delete(visiting, definition.id)
	return value, resolved
}

func (flow directCodingBrowserOutputDataflow) projectOutputProperty(
	receiver *treesitter.Node,
	key string,
	visiting map[uintptr]struct{},
) (*treesitter.Node, bool) {
	container, structural := flow.resolveOutputContainer(receiver, visiting)
	if !structural {
		return nil, false
	}
	switch container.Kind() {
	case "object":
		return flow.projectObjectProperty(container, key), true
	case "array":
		return projectDirectCodingBrowserArrayElement(container, key), true
	default:
		return nil, true
	}
}

func (flow directCodingBrowserOutputDataflow) resolveOutputContainer(
	node *treesitter.Node,
	visiting map[uintptr]struct{},
) (*treesitter.Node, bool) {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return nil, false
	}
	if node.Kind() == "object" || node.Kind() == "array" {
		return node, true
	}
	if node.Kind() == "call_expression" {
		projection, projected := flow.projectLocalOutputCall(node)
		if !projected {
			return nil, false
		}
		if _, cycle := visiting[projection.callable.Id()]; cycle {
			return nil, true
		}
		visiting[projection.callable.Id()] = struct{}{}
		container, structural := flow.resolveOutputContainer(projection.value, visiting)
		delete(visiting, projection.callable.Id())
		return container, structural
	}
	if node.Kind() == "member_expression" || node.Kind() == "subscript_expression" {
		selected, projected := flow.projectMemberValue(node)
		if !projected || selected == nil {
			return nil, projected
		}
		return flow.resolveOutputContainer(selected, visiting)
	}
	if node.Kind() != "identifier" {
		return nil, false
	}
	definition := flow.resolve(flow.text(node), node)
	if definition == nil || definition.kind != directCodingBrowserOutputAlias {
		return nil, false
	}
	if _, cycle := visiting[definition.id]; cycle {
		return nil, true
	}
	visiting[definition.id] = struct{}{}
	container, structural := flow.resolveOutputContainer(definition.value, visiting)
	delete(visiting, definition.id)
	return container, structural
}

func (flow directCodingBrowserOutputDataflow) projectObjectProperty(
	object *treesitter.Node,
	key string,
) *treesitter.Node {
	var selected *treesitter.Node
	uncertain := false
	for index := uint(0); index < object.NamedChildCount(); index++ {
		property := object.NamedChild(index)
		if property == nil || property.Kind() == "comment" {
			continue
		}
		if property.Kind() == "spread_element" {
			selected, uncertain = nil, true
			continue
		}
		name, value, resolved := directCodingBrowserOutputObjectEntry(property, flow.source)
		if !resolved {
			selected, uncertain = nil, true
			continue
		}
		if name == key {
			selected, uncertain = value, false
		}
	}
	if uncertain {
		return nil
	}
	return selected
}

func directCodingBrowserOutputObjectEntry(
	node *treesitter.Node,
	source []byte,
) (string, *treesitter.Node, bool) {
	if node == nil {
		return "", nil, false
	}
	switch node.Kind() {
	case "pair":
		name, resolved := directCodingBrowserRuntimePatternProperty(
			source, node.ChildByFieldName("key"),
		)
		return name, node.ChildByFieldName("value"), resolved
	case "shorthand_property_identifier":
		return directCodingBrowserRuntimeNodeText(source, node), node, true
	case "method_definition":
		name := node.ChildByFieldName("name")
		if name == nil && node.NamedChildCount() > 0 {
			name = node.NamedChild(0)
		}
		value, resolved := directCodingBrowserRuntimePatternProperty(source, name)
		return value, node, resolved
	default:
		return "", nil, false
	}
}

func projectDirectCodingBrowserArrayElement(
	array *treesitter.Node,
	key string,
) *treesitter.Node {
	target, err := strconv.ParseUint(key, 10, 32)
	if err != nil || strconv.FormatUint(target, 10) != key {
		return nil
	}
	position := uint64(0)
	uncertain := false
	for index := uint(0); index < array.ChildCount(); index++ {
		child := array.Child(index)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "[", "]", "comment":
			continue
		case ",":
			position++
		case "spread_element":
			if position <= target {
				uncertain = true
			}
		default:
			if position == target && !uncertain {
				return child
			}
		}
	}
	return nil
}
