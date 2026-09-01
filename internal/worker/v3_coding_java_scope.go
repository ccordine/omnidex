package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func validateDirectCodingJavaScope(
	input assemblyline.FragmentGenerationInput,
	body string,
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
	replaceableExternal, err := directCodingJavaPermittedValueAuthorities(input)
	if err != nil {
		return err
	}
	return javaInspectFragmentScope(
		input, body, root, content, authorities, methods, receiverMethods, bindings,
		replaceableExternal,
	)
}

func javaInspectFragmentScope(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	authorities map[string]struct{},
	methods map[javaMethodKey]struct{},
	receiverMethods map[string]map[javaMethodKey]javaMethodAuthority,
	bindings map[string]string,
	replaceableExternal map[string]struct{},
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
		if defect := javaMethodInvocationCorrection(
			node, directCodingTreeRoot(node), source,
			authorities, methods, receiverMethods, bindings,
		); defect != nil {
			if defect.target == nil {
				return defect.cause
			}
			choices, err := directCodingJavaTokenChoices(
				input, body, defect.target, source, defect.candidates,
			)
			if err != nil {
				return fmt.Errorf("enumerate exact Java invocation correction: %w", err)
			}
			return directCodingIdentifierNodeError(
				defect.target, defect.question, choices, defect.cause,
			)
		}
	case "method_reference":
		return fmt.Errorf("Java fragment cannot introduce method-reference authority")
	case "type_identifier":
		name := javaNodeText(node, source)
		if _, forbidden := javaForbiddenAuthority[name]; forbidden {
			choices, err := directCodingJavaTypeChoices(
				input, body, node, source, receiverMethods,
			)
			if err != nil {
				return fmt.Errorf("enumerate exact Java type correction: %w", err)
			}
			return directCodingIdentifierNodeError(
				node,
				"Which available type should replace this unavailable type reference?",
				choices,
				fmt.Errorf("Java fragment uses forbidden authority %s", name),
			)
		}
		if _, allowed := authorities[name]; !allowed {
			choices, err := directCodingJavaTypeChoices(
				input, body, node, source, receiverMethods,
			)
			if err != nil {
				return fmt.Errorf("enumerate exact Java type correction: %w", err)
			}
			return directCodingIdentifierNodeError(
				node,
				"Which available type should replace this unresolved type reference?",
				choices,
				fmt.Errorf("Java fragment references undeclared direct type %s", name),
			)
		}
	case "identifier":
		name := javaNodeText(node, source)
		bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
		failedStart := int(node.StartByte()) - bodyStart
		failedEnd := int(node.EndByte()) - bodyStart
		if _, forbidden := javaForbiddenAuthority[name]; forbidden {
			replacements, replacementErr := directCodingJavaIdentifierChoices(
				input, body, failedStart, failedEnd,
				name, directCodingTreeRoot(node), source, node, replaceableExternal,
			)
			if replacementErr != nil {
				return replacementErr
			}
			return directCodingIdentifierNodeError(
				node,
				"Which available value has the meaning required at this unavailable reference?",
				replacements,
				fmt.Errorf("Java fragment uses forbidden authority %s", name),
			)
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
		replacements, replacementErr := directCodingJavaIdentifierChoices(
			input, body, failedStart, failedEnd,
			name, directCodingTreeRoot(node), source, node, replaceableExternal,
		)
		if replacementErr != nil {
			return replacementErr
		}
		return directCodingIdentifierNodeError(
			node,
			"Which available value has the meaning required at this unresolved reference?",
			replacements,
			fmt.Errorf("Java fragment references undeclared direct symbol %s", name),
		)
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if err := javaInspectFragmentScope(
			input, body, node.Child(index), source, authorities, methods, receiverMethods, bindings,
			replaceableExternal,
		); err != nil {
			return err
		}
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
