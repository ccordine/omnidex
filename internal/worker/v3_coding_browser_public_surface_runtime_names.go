package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func directCodingBrowserRuntimeReferenceName(
	source []byte,
	node *treesitter.Node,
) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "identifier", "shorthand_property_identifier":
		return directCodingBrowserRuntimeNodeText(source, node), true
	default:
		return "", false
	}
}

func directCodingBrowserRuntimeProperty(
	source []byte,
	node *treesitter.Node,
) (string, bool, bool) {
	if node == nil {
		return "", false, false
	}
	switch node.Kind() {
	case "member_expression":
		property := node.ChildByFieldName("property")
		if property == nil {
			return "", false, true
		}
		return directCodingBrowserRuntimeNodeText(source, property), true, true
	case "subscript_expression":
		name, resolved := javaScriptStaticPropertyName(source, node.ChildByFieldName("index"))
		return name, resolved, true
	default:
		return "", false, false
	}
}
