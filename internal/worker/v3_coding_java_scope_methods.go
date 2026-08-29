package worker

import treesitter "github.com/tree-sitter/go-tree-sitter"

func javaMethodInvocationArity(node *treesitter.Node) int {
	arguments := node.ChildByFieldName("arguments")
	if arguments == nil {
		return 0
	}
	return int(arguments.NamedChildCount())
}

func javaLookupMethodAuthority(
	owner string,
	key javaMethodKey,
	receivers map[string]map[javaMethodKey]javaMethodAuthority,
) (javaMethodAuthority, bool) {
	if method, exists := receivers[owner][key]; exists {
		return method, true
	}
	method, exists := javaPureMethods[owner][key]
	return method, exists
}

func javaExpressionOwner(
	node *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	receivers map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "identifier", "type_identifier":
		name := javaNodeText(node, source)
		if owner, bound := bindings[name]; bound {
			return owner, false
		}
		if _, allowed := authorities[name]; allowed {
			return name, true
		}
		return "", false
	case "string_literal":
		return "String", false
	case "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal", "binary_integer_literal":
		return "Integer", false
	case "decimal_floating_point_literal", "hex_floating_point_literal":
		return "Double", false
	case "true", "false":
		return "Boolean", false
	case "object_creation_expression", "cast_expression":
		return javaDeclaredTypeOwner(node.ChildByFieldName("type"), source), false
	case "method_invocation":
		return javaMethodInvocationReturnOwner(node, source, authorities, receivers, bindings), false
	case "parenthesized_expression":
		if node.NamedChildCount() == 1 {
			return javaExpressionOwner(
				node.NamedChild(0), source, authorities, receivers, bindings,
			)
		}
	}
	return "", false
}

func javaMethodInvocationReturnOwner(
	node *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	receivers map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) string {
	name := node.ChildByFieldName("name")
	object := node.ChildByFieldName("object")
	if name == nil || object == nil {
		return ""
	}
	owner, staticReceiver := javaExpressionOwner(
		object, source, authorities, receivers, bindings,
	)
	method, exists := javaLookupMethodAuthority(
		owner,
		javaMethodKey{Name: javaNodeText(name, source), Arity: javaMethodInvocationArity(node)},
		receivers,
	)
	if !exists || method.Static != staticReceiver {
		return ""
	}
	return method.ReturnOwner
}
