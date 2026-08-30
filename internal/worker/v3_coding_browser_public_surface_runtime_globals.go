package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func directCodingBrowserRuntimeGlobalReferenceEscapes(
	node *treesitter.Node,
	name string,
) bool {
	if directCodingBrowserRuntimeGlobalValueConstant(name) {
		return false
	}
	return !directCodingBrowserRuntimeExpressionHasDirectUse(node)
}

func directCodingBrowserRuntimeGlobalPropertyEscapes(
	node *treesitter.Node,
	name string,
	resolved bool,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	root, global := directCodingBrowserRuntimePermittedGlobalRoot(
		node, source, bindings,
	)
	if !global {
		return false
	}
	if resolved && directCodingBrowserRuntimeStableGlobalProperty(root, name) {
		return false
	}
	if resolved && name == "valueOf" {
		return true
	}
	return !directCodingBrowserRuntimeExpressionHasDirectUse(node)
}

func directCodingBrowserRuntimeExpressionHasDirectUse(node *treesitter.Node) bool {
	current := directCodingBrowserRuntimeOuterTransparentExpression(node)
	if current == nil || current.Parent() == nil {
		return false
	}
	parent := current.Parent()
	switch parent.Kind() {
	case "call_expression":
		return directCodingBrowserRuntimeSameNode(
			parent.ChildByFieldName("function"), current,
		)
	case "new_expression":
		constructor := parent.ChildByFieldName("constructor")
		if constructor == nil && parent.NamedChildCount() > 0 {
			constructor = parent.NamedChild(0)
		}
		return directCodingBrowserRuntimeSameNode(constructor, current)
	case "member_expression", "subscript_expression":
		return directCodingBrowserRuntimeSameNode(
			parent.ChildByFieldName("object"), current,
		)
	default:
		return false
	}
}

func directCodingBrowserRuntimePermittedGlobalRoot(
	node *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) (string, bool) {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "identifier", "shorthand_property_identifier":
		name := directCodingBrowserRuntimeNodeText(source, node)
		return name, directCodingBrowserRuntimeGlobalPermitted(name) &&
			!bindings.binds(name, node)
	case "member_expression", "subscript_expression":
		return directCodingBrowserRuntimePermittedGlobalRoot(
			node.ChildByFieldName("object"), source, bindings,
		)
	default:
		return "", false
	}
}

func directCodingBrowserRuntimeGlobalValueConstant(name string) bool {
	switch name {
	case "undefined", "NaN", "Infinity":
		return true
	default:
		return false
	}
}

func directCodingBrowserRuntimeStableGlobalProperty(root, name string) bool {
	switch root {
	case "Math":
		switch name {
		case "E", "LN10", "LN2", "LOG10E", "LOG2E", "PI", "SQRT1_2", "SQRT2":
			return true
		}
	case "Number":
		switch name {
		case "EPSILON", "MAX_SAFE_INTEGER", "MIN_SAFE_INTEGER", "MAX_VALUE", "MIN_VALUE",
			"NaN", "NEGATIVE_INFINITY", "POSITIVE_INFINITY":
			return true
		}
	case "Symbol":
		switch name {
		case "asyncDispose", "asyncIterator", "dispose", "hasInstance",
			"isConcatSpreadable", "iterator", "match", "matchAll", "replace",
			"search", "species", "split", "toPrimitive", "toStringTag", "unscopables":
			return true
		}
	}
	return false
}

func directCodingBrowserRuntimeSameNode(left, right *treesitter.Node) bool {
	return left != nil && right != nil && left.Id() == right.Id()
}
