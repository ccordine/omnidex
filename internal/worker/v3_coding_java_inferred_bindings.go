package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

// Java requires each inferred local's initializer to precede its uses. Resolve
// that source order from declarations already owned by the parser and API map.
func javaResolveInferredBindings(node *treesitter.Node, source []byte, authorities map[string]struct{}, receivers map[string]map[javaMethodKey]javaMethodAuthority, bindings map[string]string) {
	if node == nil {
		return
	}
	if node.Kind() == "variable_declarator" {
		parent, name := node.Parent(), node.ChildByFieldName("name")
		if parent != nil && name != nil && javaDeclaredTypeOwner(parent.ChildByFieldName("type"), source) == "var" {
			owner, _ := javaExpressionOwner(node.ChildByFieldName("value"), source, authorities, receivers, bindings)
			bindings[javaNodeText(name, source)] = owner
		}
	}
	for _, child := range javaNamedSyntaxChildren(node) {
		javaResolveInferredBindings(child, source, authorities, receivers, bindings)
	}
}
