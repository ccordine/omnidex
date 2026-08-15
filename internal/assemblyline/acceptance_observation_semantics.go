package assemblyline

import treesitter "github.com/tree-sitter/go-tree-sitter"

type acceptanceCandidateSemantics struct {
	primary    string
	operations []string
	literals   []AcceptanceObservationLiteral
	trusted    bool
}

func collectAcceptanceCandidateSemantics(
	call *treesitter.Node,
	operation string,
	source []byte,
) acceptanceCandidateSemantics {
	result := acceptanceCandidateSemantics{
		primary: operation, operations: []string{},
		literals: []AcceptanceObservationLiteral{}, trusted: true,
	}
	if call == nil || call.Kind() != "call_expression" {
		result.trusted = false
		return result
	}
	if matcherOperation(operation) {
		expectCall, modifiers, trusted := acceptanceMatcherChain(call, source)
		result.trusted = trusted
		for _, modifier := range modifiers {
			result.operations = appendAcceptanceOperation(
				result.operations, "matcher_modifier:"+modifier,
			)
		}
		if expectCall != nil {
			walkAcceptanceSemanticExpression(
				expectCall.ChildByFieldName("arguments"), source, &result,
			)
		}
	}
	walkAcceptanceSemanticExpression(call.ChildByFieldName("arguments"), source, &result)
	return result
}

func acceptanceMatcherChain(
	call *treesitter.Node,
	source []byte,
) (*treesitter.Node, []string, bool) {
	callee := call.ChildByFieldName("function")
	if callee == nil || callee.Kind() != "member_expression" {
		return nil, nil, false
	}
	current := callee.ChildByFieldName("object")
	modifiers := []string{}
	trusted := true
	for current != nil && current.Kind() == "member_expression" {
		property := current.ChildByFieldName("property")
		if property == nil || !acceptanceMatcherModifier(property.Utf8Text(source)) {
			trusted = false
		} else {
			modifiers = append([]string{property.Utf8Text(source)}, modifiers...)
		}
		current = current.ChildByFieldName("object")
	}
	if !isBareExpectCall(current, source) {
		return nil, modifiers, false
	}
	return current, modifiers, trusted
}

func walkAcceptanceSemanticExpression(
	node *treesitter.Node,
	source []byte,
	result *acceptanceCandidateSemantics,
) {
	if node == nil || node.Kind() == "comment" {
		return
	}
	if node.Kind() == "call_expression" {
		return
	}
	if node.Kind() == "template_string" {
		literal, static := acceptanceLiteral(node, source)
		if !static {
			result.trusted = false
			result.literals = append(result.literals, AcceptanceObservationLiteral{
				Kind: "interpolated_template", Value: "<dynamic>",
			})
		} else {
			result.literals = append(result.literals, literal)
		}
		return
	}
	if literal, ok := acceptanceLiteral(node, source); ok {
		result.literals = append(result.literals, literal)
		return
	}
	if node.Kind() == "member_expression" {
		property := node.ChildByFieldName("property")
		if property == nil || !acceptancePublicObservationProperty(property.Utf8Text(source)) {
			result.trusted = false
		} else {
			result.operations = appendAcceptanceOperation(
				result.operations, "public_observation:"+property.Utf8Text(source),
			)
		}
		walkAcceptanceSemanticExpression(node.ChildByFieldName("object"), source, result)
		return
	}
	if node.Kind() == "subscript_expression" {
		result.operations = appendAcceptanceOperation(
			result.operations, "subscript_observation:index",
		)
		walkAcceptanceSemanticExpression(node.ChildByFieldName("object"), source, result)
		index := node.ChildByFieldName("index")
		if _, ok := acceptanceLiteral(index, source); !ok {
			result.trusted = false
		}
		walkAcceptanceSemanticExpression(index, source, result)
		return
	}
	if node.Kind() == "pair" {
		key := node.ChildByFieldName("key")
		name := ""
		if key != nil && (key.Kind() == "property_identifier" || key.Kind() == "string") {
			name = key.Utf8Text(source)
			if key.Kind() == "string" {
				name = decodeAcceptanceString(name)
			}
		}
		if operation := acceptanceSemanticFieldOperation(result.primary, name); operation == "" {
			result.trusted = false
		} else {
			result.operations = appendAcceptanceOperation(result.operations, operation)
		}
		walkAcceptanceSemanticExpression(node.ChildByFieldName("value"), source, result)
		return
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		walkAcceptanceSemanticExpression(node.NamedChild(index), source, result)
	}
}

func acceptanceSemanticFieldOperation(primary string, name string) string {
	if prefixOperation(primary, "testing_library_query:") {
		switch name {
		case "busy", "checked", "current", "description", "exact", "expanded", "hidden",
			"level", "name", "pressed", "selected", "selector", "value":
			return "query_option:" + name
		}
	}
	if prefixOperation(primary, "fire_event:") {
		switch name {
		case "altKey", "bubbles", "button", "buttons", "cancelable", "charCode", "checked",
			"clientX", "clientY", "code", "composed", "ctrlKey", "data", "detail", "files",
			"height", "inputType", "isComposing", "isPrimary", "key", "keyCode", "location",
			"metaKey", "movementX", "movementY", "name", "pageX", "pageY", "pointerId",
			"pointerType", "pressure", "repeat", "screenX", "screenY", "selectionEnd",
			"selectionStart", "shiftKey", "tangentialPressure", "target", "tiltX", "tiltY",
			"twist", "value", "which", "width":
			return "event_payload:" + name
		}
	}
	return ""
}

func matcherOperation(operation string) bool {
	return prefixOperation(operation, "expect_matcher:")
}

func prefixOperation(operation string, prefix string) bool {
	return len(operation) > len(prefix) && operation[:len(prefix)] == prefix
}

func isBareExpectCall(node *treesitter.Node, source []byte) bool {
	if node == nil || node.Kind() != "call_expression" {
		return false
	}
	callee := node.ChildByFieldName("function")
	return callee != nil && callee.Kind() == "identifier" && callee.Utf8Text(source) == "expect"
}

func acceptanceMatcherModifier(value string) bool {
	switch value {
	case "not":
		return true
	default:
		return false
	}
}

func acceptancePublicObservationProperty(value string) bool {
	switch value {
	case "checked", "disabled", "length", "selected", "selectedIndex", "textContent", "value":
		return true
	default:
		return false
	}
}

func appendAcceptanceOperation(values []string, operation string) []string {
	for _, value := range values {
		if value == operation {
			return values
		}
	}
	return append(values, operation)
}
