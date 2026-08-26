package worker

import (
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func javaPermittedAuthorities(
	input assemblyline.FragmentGenerationInput,
) (
	map[string]struct{},
	map[javaMethodKey]struct{},
	map[string]map[javaMethodKey]javaMethodAuthority,
) {
	authorities := javaTaskNeutralAuthorities()
	methods := make(map[javaMethodKey]struct{})
	receivers := make(map[string]map[javaMethodKey]javaMethodAuthority)
	values := append([]string(nil), input.Capabilities...)
	values = append(values, input.PermittedSymbols...)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if javaSourceIdentifier(trimmed) {
			authorities[trimmed] = struct{}{}
			continue
		}
		javaCollectAPIDeclarations([]byte(trimmed), authorities, methods, receivers)
	}
	return authorities, methods, receivers
}

func javaCollectAPIDeclarations(
	source []byte,
	authorities map[string]struct{},
	methods map[javaMethodKey]struct{},
	receivers map[string]map[javaMethodKey]javaMethodAuthority,
) {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
		return
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return
	}
	defer tree.Close()
	var visit func(*treesitter.Node, string)
	visit = func(node *treesitter.Node, owner string) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			if name := node.ChildByFieldName("name"); name != nil {
				owner = javaNodeText(name, source)
				authorities[owner] = struct{}{}
			}
		case "method_declaration":
			name := node.ChildByFieldName("name")
			if name != nil {
				key := javaMethodKey{
					Name: javaNodeText(name, source), Arity: javaDeclarationArity(node),
				}
				if owner != "" {
					if receivers[owner] == nil {
						receivers[owner] = make(map[javaMethodKey]javaMethodAuthority)
					}
					receivers[owner][key] = javaMethodAuthority{
						ReturnOwner: javaDeclaredTypeOwner(
							node.ChildByFieldName("type"), source,
						),
						Static: javaMethodDeclarationStatic(node, source),
					}
				} else {
					methods[key] = struct{}{}
				}
			}
		case "variable_declarator":
			if parent := node.Parent(); parent != nil && parent.Kind() == "field_declaration" {
				if name := node.ChildByFieldName("name"); name != nil {
					authorities[javaNodeText(name, source)] = struct{}{}
				}
			}
		}
		for index := uint(0); index < node.ChildCount(); index++ {
			visit(node.Child(index), owner)
		}
	}
	visit(tree.RootNode(), "")
}

func javaCollectFragmentBindings(
	node *treesitter.Node,
	source []byte,
	bindings map[string]string,
	methods map[javaMethodKey]struct{},
) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "method_declaration":
		if name := node.ChildByFieldName("name"); name != nil {
			methods[javaMethodKey{
				Name: javaNodeText(name, source), Arity: javaDeclarationArity(node),
			}] = struct{}{}
		}
	case "formal_parameter", "spread_parameter", "catch_formal_parameter", "type_pattern":
		javaCollectTypedBinding(node, source, bindings)
	case "variable_declarator":
		name := node.ChildByFieldName("name")
		parent := node.Parent()
		if name != nil && parent != nil {
			bindings[javaNodeText(name, source)] = javaDeclaredTypeOwner(
				parent.ChildByFieldName("type"), source,
			)
		}
	case "enhanced_for_statement", "instanceof_expression":
		javaCollectTypedBinding(node, source, bindings)
	case "record_pattern", "record_pattern_component", "lambda_expression":
		javaCollectUntypedIdentifiers(node.ChildByFieldName("parameters"), source, bindings)
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		javaCollectFragmentBindings(node.Child(index), source, bindings, methods)
	}
}

func javaCollectTypedBinding(
	node *treesitter.Node,
	source []byte,
	bindings map[string]string,
) {
	name := node.ChildByFieldName("name")
	if name == nil {
		return
	}
	owner := javaDeclaredTypeOwner(node.ChildByFieldName("type"), source)
	if owner == "" {
		for index := uint(0); index < node.NamedChildCount(); index++ {
			child := node.NamedChild(index)
			if child == nil || child.Id() == name.Id() {
				continue
			}
			if owner = javaDeclaredTypeOwner(child, source); owner != "" {
				break
			}
		}
	}
	bindings[javaNodeText(name, source)] = owner
}

func javaCollectUntypedIdentifiers(
	node *treesitter.Node,
	source []byte,
	bindings map[string]string,
) {
	if node == nil {
		return
	}
	if node.Kind() == "identifier" {
		bindings[javaNodeText(node, source)] = ""
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		javaCollectUntypedIdentifiers(node.Child(index), source, bindings)
	}
}

func javaDeclaredTypeOwner(node *treesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "type_identifier":
		return javaNodeText(node, source)
	case "integral_type":
		if javaNodeText(node, source) == "int" {
			return "Integer"
		}
		return "Number"
	case "floating_point_type":
		if javaNodeText(node, source) == "double" {
			return "Double"
		}
		return "Float"
	case "boolean_type":
		return "Boolean"
	case "void_type":
		return ""
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if owner := javaDeclaredTypeOwner(node.Child(index), source); owner != "" {
			return owner
		}
	}
	return ""
}

func javaDeclarationArity(node *treesitter.Node) int {
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return 0
	}
	return int(parameters.NamedChildCount())
}

func javaMethodDeclarationStatic(node *treesitter.Node, source []byte) bool {
	name := node.ChildByFieldName("name")
	if name == nil {
		return false
	}
	prefix := string(source[node.StartByte():name.StartByte()])
	for _, field := range strings.Fields(prefix) {
		if field == "static" {
			return true
		}
	}
	return false
}
