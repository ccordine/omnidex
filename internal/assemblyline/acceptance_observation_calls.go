package assemblyline

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func acceptanceCallOperation(call *treesitter.Node, source []byte) string {
	callee := call.ChildByFieldName("function")
	if callee == nil {
		return "untrusted_call"
	}
	if callee.Kind() == "identifier" {
		name := callee.Utf8Text(source)
		if name == "expect" {
			if expectCallHasMatcher(call) {
				return ""
			}
			return "harness_call:expect"
		}
		if acceptanceHarnessCall(name) {
			return "harness_call:" + name
		}
		return "untrusted_call"
	}
	if callee.Kind() != "member_expression" {
		return "untrusted_call"
	}
	property := callee.ChildByFieldName("property")
	object := callee.ChildByFieldName("object")
	if property == nil {
		return "untrusted_call"
	}
	name := property.Utf8Text(source)
	if isTestingLibraryQuery(name) {
		if object == nil || object.Kind() != "identifier" || object.Utf8Text(source) != "screen" {
			return "untrusted_call"
		}
		return "testing_library_query:" + name
	}
	if object != nil && object.Kind() == "identifier" && object.Utf8Text(source) == "fireEvent" {
		return "fire_event:" + name
	}
	if object != nil && containsExpectCall(object, source) {
		return "expect_matcher:" + name
	}
	return "untrusted_call"
}

func acceptanceHarnessCall(name string) bool {
	switch name {
	case "waitFor":
		return true
	default:
		return false
	}
}

func expectCallHasMatcher(call *treesitter.Node) bool {
	for ancestor := call.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		if ancestor.Kind() == "call_expression" {
			callee := ancestor.ChildByFieldName("function")
			return callee != nil && containsNodeID(callee, call.Id())
		}
		if ancestor.Kind() != "member_expression" {
			return false
		}
	}
	return false
}

func containsNodeID(node *treesitter.Node, id uintptr) bool {
	if node == nil {
		return false
	}
	if node.Id() == id {
		return true
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsNodeID(node.NamedChild(index), id) {
			return true
		}
	}
	return false
}

func containsExpectCall(node *treesitter.Node, source []byte) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "call_expression" {
		callee := node.ChildByFieldName("function")
		if callee != nil && callee.Kind() == "identifier" && callee.Utf8Text(source) == "expect" {
			return true
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsExpectCall(node.NamedChild(index), source) {
			return true
		}
	}
	return false
}

func isTestingLibraryQuery(name string) bool {
	for _, prefix := range []string{"getBy", "getAllBy", "findBy", "findAllBy", "queryBy", "queryAllBy"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		for _, registered := range []string{
			"AltText", "DisplayValue", "LabelText", "PlaceholderText", "Role", "Text", "Title",
		} {
			if suffix == registered {
				return true
			}
		}
	}
	return false
}

func acceptanceRegisteredMatcher(name string) bool {
	switch name {
	case "toBe", "toBeChecked", "toBeDisabled", "toBeEnabled", "toBeEmptyDOMElement",
		"toBeInTheDocument", "toBeInvalid", "toBeRequired", "toBeTruthy", "toBeValid",
		"toBeVisible", "toContainElement", "toContainHTML", "toEqual", "toHaveAccessibleDescription",
		"toHaveAccessibleName", "toHaveAttribute", "toHaveClass", "toHaveDisplayValue",
		"toHaveFocus", "toHaveFormValues", "toHaveStyle", "toHaveTextContent", "toHaveValue",
		"toHaveLength":
		return true
	default:
		return false
	}
}

func acceptanceRegisteredEvent(name string) bool {
	switch name {
	case "blur", "change", "click", "doubleClick", "drag", "drop", "focus", "input",
		"keyDown", "keyPress", "keyUp", "mouseDown", "mouseMove", "mouseUp", "pointerDown",
		"pointerMove", "pointerUp", "submit":
		return true
	default:
		return false
	}
}

func acceptanceOperator(node *treesitter.Node, source []byte) string {
	switch node.Kind() {
	case "binary_expression", "unary_expression", "ternary_expression", "assignment_expression",
		"augmented_assignment_expression", "update_expression":
		operator := node.ChildByFieldName("operator")
		if operator != nil {
			return operator.Utf8Text(source)
		}
	}
	return ""
}
