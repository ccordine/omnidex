package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserEventBinding struct {
	name          string
	start         uint
	end           uint
	declarationID uintptr
}

type directCodingBrowserEventBindings []directCodingBrowserEventBinding

func (bindings directCodingBrowserEventBindings) reference(
	node *treesitter.Node,
	source []byte,
) bool {
	if node == nil || node.Kind() != "identifier" {
		return false
	}
	name := directCodingBrowserRuntimeNodeText(source, node)
	for _, binding := range bindings {
		if binding.name == name && node.Id() != binding.declarationID &&
			node.StartByte() >= binding.start && node.EndByte() <= binding.end {
			return true
		}
	}
	return false
}

func collectDirectCodingBrowserEventBindings(
	root *treesitter.Node,
	source []byte,
) (directCodingBrowserEventBindings, error) {
	definitions := make(map[string][]*treesitter.Node)
	handlers := make([]*treesitter.Node, 0)
	var collect func(*treesitter.Node)
	collect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "function_declaration":
			if name := node.ChildByFieldName("name"); name != nil {
				key := directCodingBrowserRuntimeNodeText(source, name)
				definitions[key] = append(definitions[key], node)
			}
		case "variable_declarator":
			name := node.ChildByFieldName("name")
			value := node.ChildByFieldName("value")
			if name != nil && name.Kind() == "identifier" {
				if function := directCodingBrowserBoundEventFunction(value, source); function != nil {
					key := directCodingBrowserRuntimeNodeText(source, name)
					definitions[key] = append(definitions[key], function)
				}
			}
		case "jsx_attribute":
			name := node.NamedChild(0)
			if name != nil && directCodingBrowserEventHandlerAttribute(
				directCodingBrowserRuntimeNodeText(source, name),
			) {
				if node.NamedChildCount() > 1 {
					handlers = append(handlers, node.NamedChild(1))
				} else {
					handlers = append(handlers, nil)
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			collect(node.NamedChild(index))
		}
	}
	collect(root)
	functions := make(map[uintptr]*treesitter.Node)
	for _, handler := range handlers {
		expression := directCodingBrowserUnwrapRuntimeExpression(handler)
		if expression != nil && expression.Kind() == "jsx_expression" && expression.NamedChildCount() == 1 {
			expression = directCodingBrowserUnwrapRuntimeExpression(expression.NamedChild(0))
		}
		function := directCodingBrowserBoundEventFunction(expression, source)
		if function == nil && expression != nil && expression.Kind() == "identifier" {
			matches := definitions[directCodingBrowserRuntimeNodeText(source, expression)]
			if len(matches) == 1 {
				function = matches[0]
			}
		}
		if function == nil {
			return nil, fmt.Errorf("browser public surface requires one statically bound event handler")
		}
		functions[function.Id()] = function
	}
	bindings := make(directCodingBrowserEventBindings, 0, len(functions))
	for _, function := range functions {
		parameter, err := directCodingBrowserEventParameter(function)
		if err != nil {
			return nil, err
		}
		if parameter == nil {
			continue
		}
		bindings = append(bindings, directCodingBrowserEventBinding{
			name:  directCodingBrowserRuntimeNodeText(source, parameter),
			start: function.StartByte(), end: function.EndByte(), declarationID: parameter.Id(),
		})
	}
	return bindings, nil
}

func directCodingBrowserBoundEventFunction(
	node *treesitter.Node,
	source []byte,
) *treesitter.Node {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "arrow_function", "function_expression", "function_declaration":
		return node
	case "call_expression":
		callee := node.ChildByFieldName("function")
		if callee == nil || directCodingBrowserRuntimeNodeText(source, callee) != "useCallback" {
			return nil
		}
		arguments := node.ChildByFieldName("arguments")
		if arguments != nil && arguments.NamedChildCount() > 0 {
			return directCodingBrowserBoundEventFunction(arguments.NamedChild(0), source)
		}
	}
	return nil
}

func directCodingBrowserEventParameter(function *treesitter.Node) (*treesitter.Node, error) {
	parameters := function.ChildByFieldName("parameters")
	if parameters == nil {
		parameters = function.ChildByFieldName("parameter")
	}
	if parameters == nil {
		return nil, nil
	}
	parameter := parameters
	if parameters.Kind() == "formal_parameters" {
		if parameters.NamedChildCount() == 0 {
			return nil, nil
		}
		parameter = parameters.NamedChild(0)
	}
	if parameter.Kind() == "required_parameter" || parameter.Kind() == "optional_parameter" {
		pattern := parameter.ChildByFieldName("pattern")
		if pattern == nil && parameter.NamedChildCount() > 0 {
			pattern = parameter.NamedChild(0)
		}
		parameter = pattern
	}
	if parameter == nil || parameter.Kind() != "identifier" {
		return nil, fmt.Errorf("browser public surface rejects destructured or dynamic event parameters")
	}
	return parameter, nil
}

func directCodingBrowserEventReferenceIsPropertyObject(node *treesitter.Node) bool {
	current := directCodingBrowserRuntimeOuterTransparentExpression(node)
	if current == nil || current.Parent() == nil {
		return false
	}
	object := current.Parent().ChildByFieldName("object")
	return object != nil && object.Id() == current.Id()
}

func directCodingBrowserPatternAliasesEventTarget(
	node *treesitter.Node,
	source []byte,
	eventBindings directCodingBrowserEventBindings,
) bool {
	if node == nil || node.Kind() != "pair_pattern" {
		return false
	}
	key := node.ChildByFieldName("key")
	value := node.ChildByFieldName("value")
	name, resolved := directCodingBrowserRuntimePatternProperty(source, key)
	return resolved && (name == "target" || name == "currentTarget") &&
		directCodingBrowserExpressionIsEventRoot(value, source, eventBindings)
}

func directCodingBrowserRuntimePatternProperty(
	source []byte,
	node *treesitter.Node,
) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "property_identifier", "shorthand_property_identifier_pattern":
		return directCodingBrowserRuntimeNodeText(source, node), true
	case "computed_property_name":
		if node.NamedChildCount() == 1 {
			return javaScriptStaticPropertyName(source, node.NamedChild(0))
		}
	}
	return javaScriptStaticPropertyName(source, node)
}

func directCodingBrowserExpressionContainsEventRoot(
	node *treesitter.Node,
	source []byte,
	eventBindings directCodingBrowserEventBindings,
) bool {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return false
	}
	if directCodingBrowserExpressionIsEventRoot(node, source, eventBindings) {
		return true
	}
	if node.Kind() != "member_expression" && node.Kind() != "subscript_expression" {
		return false
	}
	return directCodingBrowserExpressionContainsEventRoot(
		node.ChildByFieldName("object"), source, eventBindings,
	)
}
