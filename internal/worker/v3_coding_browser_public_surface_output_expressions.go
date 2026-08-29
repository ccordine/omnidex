package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func (extractor *directCodingBrowserPublicSurfaceExtractor) outputExpressionIsRuntimeDerived(
	expression *treesitter.Node,
) bool {
	return extractor.outputExpressionHasRuntimeReference(expression)
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) outputExpressionHasRuntimeReference(
	node *treesitter.Node,
) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier":
		return !directCodingBrowserStaticOutputIdentifier(extractor.nodeText(node))
	case "shorthand_property_identifier", "this":
		return true
	case "property_identifier", "private_property_identifier", "type_identifier",
		"predefined_type", "comment", "number", "string", "regex", "true",
		"false", "null", "undefined":
		return false
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if extractor.outputExpressionHasRuntimeReference(node.NamedChild(index)) {
			return true
		}
	}
	return false
}

func directCodingBrowserStaticOutputIdentifier(name string) bool {
	switch name {
	case "undefined", "NaN", "Infinity", "String", "Number", "Boolean", "BigInt",
		"Object", "Array", "JSON", "Math", "parseInt", "parseFloat", "isNaN",
		"isFinite":
		return true
	default:
		return false
	}
}
