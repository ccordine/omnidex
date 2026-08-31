package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func validateDirectCodingJavaScope(
	input assemblyline.FragmentGenerationInput,
	source string,
) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
		return err
	}
	content := []byte(source)
	tree := parser.Parse(content, nil)
	if tree == nil {
		return fmt.Errorf("Java scope parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("Java scope parser rejected the method declaration")
	}
	authorities, methods, receiverMethods, err := javaPermittedAuthorities(input)
	if err != nil {
		return err
	}
	bindings := make(map[string]string)
	javaCollectFragmentBindings(root, content, bindings, methods)
	return javaInspectFragmentScope(
		root, content, authorities, methods, receiverMethods, bindings,
	)
}

func javaInspectFragmentScope(
	node *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	methods map[javaMethodKey]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) error {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "package_declaration", "import_declaration", "class_declaration", "class_literal",
		"interface_declaration", "enum_declaration", "record_declaration",
		"annotation_type_declaration", "constructor_declaration":
		return fmt.Errorf("Java fragment contains forbidden %s authority", node.Kind())
	case "method_invocation":
		if err := javaValidateMethodInvocation(
			node, source, authorities, methods, receiverMethods, bindings,
		); err != nil {
			return err
		}
	case "method_reference":
		return fmt.Errorf("Java fragment cannot introduce method-reference authority")
	case "type_identifier":
		name := javaNodeText(node, source)
		if _, forbidden := javaForbiddenAuthority[name]; forbidden {
			return fmt.Errorf("Java fragment uses forbidden authority %s", name)
		}
		if _, allowed := authorities[name]; !allowed {
			return fmt.Errorf("Java fragment references undeclared direct type %s", name)
		}
	case "identifier":
		name := javaNodeText(node, source)
		if _, forbidden := javaForbiddenAuthority[name]; forbidden {
			return fmt.Errorf("Java fragment uses forbidden authority %s", name)
		}
		if javaIdentifierIsNonReference(node) {
			break
		}
		if _, bound := bindings[name]; bound {
			break
		}
		if _, allowed := authorities[name]; allowed {
			break
		}
		return fmt.Errorf("Java fragment references undeclared direct symbol %s", name)
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if err := javaInspectFragmentScope(
			node.Child(index), source, authorities, methods, receiverMethods, bindings,
		); err != nil {
			return err
		}
	}
	return nil
}

func javaValidateMethodInvocation(
	node *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	methods map[javaMethodKey]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
) error {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return fmt.Errorf("Java fragment has an unnamed method invocation")
	}
	name := javaNodeText(nameNode, source)
	key := javaMethodKey{Name: name, Arity: javaMethodInvocationArity(node)}
	if _, forbidden := javaForbiddenMethods[name]; forbidden {
		return fmt.Errorf("Java fragment calls forbidden method %s", name)
	}
	object := node.ChildByFieldName("object")
	if object == nil {
		if _, allowed := methods[key]; !allowed {
			return fmt.Errorf("Java fragment calls undeclared direct method %s", name)
		}
		return nil
	}
	owner, staticReceiver := javaExpressionOwner(
		object, source, authorities, receiverMethods, bindings,
	)
	if owner == "" {
		return fmt.Errorf("Java fragment cannot prove the owner of method %s", name)
	}
	if _, forbidden := javaForbiddenAuthority[owner]; forbidden {
		return fmt.Errorf("Java fragment uses forbidden authority %s", owner)
	}
	method, allowed := javaLookupMethodAuthority(owner, key, receiverMethods)
	if !allowed || method.Static != staticReceiver {
		return fmt.Errorf(
			"Java fragment calls undeclared method %s/%d on %s", name, key.Arity, owner,
		)
	}
	return nil
}

func javaIdentifierIsNonReference(node *treesitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	for _, field := range []string{"name", "field", "label"} {
		candidate := parent.ChildByFieldName(field)
		if candidate != nil && candidate.Id() == node.Id() {
			return true
		}
	}
	return false
}

func javaNodeText(node *treesitter.Node, source []byte) string {
	return string(source[node.StartByte():node.EndByte()])
}
