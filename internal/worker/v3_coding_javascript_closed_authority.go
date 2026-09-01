package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func directCodingJavaScriptClosedAuthorityError(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	root *treesitter.Node,
	source []byte,
	bindings javaScriptLexicalBindings,
	computedKeys map[string]string,
) error {
	switch node.Kind() {
	case "call_expression":
		callable := node.ChildByFieldName("function")
		if callable == nil || strings.TrimSpace(javaScriptNodeText(source, callable)) != "import" {
			return nil
		}
		choices, err := directCodingJavaScriptCallableChoices(
			input, body, callable, root, source, bindings,
		)
		if err != nil {
			return fmt.Errorf("enumerate exact JavaScript callable correction: %w", err)
		}
		return directCodingIdentifierNodeError(
			callable,
			"Which available callable should replace this unavailable reference?",
			choices,
			fmt.Errorf("JavaScript fragment uses forbidden dynamic import authority"),
		)
	case "meta_property":
		return fmt.Errorf("JavaScript fragment uses forbidden host metadata authority")
	case "member_expression":
		property := node.ChildByFieldName("property")
		if property == nil || !javaScriptSensitiveProperty(javaScriptNodeText(source, property)) {
			return nil
		}
		return directCodingJavaScriptPropertyError(
			input, body, property, source, computedKeys, javaScriptMemberPropertyChoice,
			"Which allowed property should replace this unavailable property?",
			fmt.Errorf(
				"JavaScript fragment uses forbidden dynamic property %s",
				javaScriptNodeText(source, property),
			),
		)
	case "pair_pattern":
		key := node.ChildByFieldName("key")
		name, resolved, authorized := javaScriptPatternProperty(source, key, computedKeys)
		if authorized && (!resolved || !javaScriptSensitiveProperty(name)) {
			return nil
		}
		if key == nil {
			return fmt.Errorf("JavaScript fragment uses unresolved destructured property authority")
		}
		target, form := javaScriptExactPropertyCorrectionTarget(
			key, javaScriptPatternPropertyChoice,
		)
		if target == nil {
			return fmt.Errorf(
				"JavaScript fragment cannot isolate one exact destructured property token",
			)
		}
		question := "Which available property should replace this unresolved property?"
		cause := fmt.Errorf("JavaScript fragment uses unresolved destructured property authority")
		if resolved {
			question = "Which allowed property should replace this unavailable property?"
			cause = fmt.Errorf("JavaScript fragment uses forbidden destructured property %s", name)
		}
		return directCodingJavaScriptPropertyError(
			input, body, target, source, computedKeys, form,
			question, cause,
		)
	case "shorthand_property_identifier_pattern":
		name := javaScriptNodeText(source, node)
		if !javaScriptSensitiveProperty(name) {
			return nil
		}
		return directCodingJavaScriptPropertyError(
			input, body, node, source, computedKeys, javaScriptPatternPropertyChoice,
			"Which allowed property should replace this unavailable property?",
			fmt.Errorf("JavaScript fragment binds forbidden destructured property %s", name),
		)
	case "subscript_expression":
		index := node.ChildByFieldName("index")
		name, resolved := javaScriptStaticPropertyName(source, index)
		if resolved && !javaScriptSensitiveProperty(name) {
			return nil
		}
		if !resolved && (javaScriptNumericSubscript(index) ||
			javaScriptCodeOwnedComputedKey(source, index, computedKeys)) {
			return nil
		}
		if index == nil {
			return fmt.Errorf("JavaScript fragment uses unresolved computed property authority")
		}
		target, form := javaScriptExactPropertyCorrectionTarget(
			index, javaScriptSubscriptPropertyChoice,
		)
		if target == nil {
			return fmt.Errorf(
				"JavaScript fragment cannot isolate one exact computed property token",
			)
		}
		question := "Which available property should replace this unresolved property?"
		cause := fmt.Errorf("JavaScript fragment uses unresolved computed property authority")
		if resolved {
			question = "Which allowed property should replace this unavailable property?"
			cause = fmt.Errorf("JavaScript fragment uses forbidden computed property %s", name)
		}
		return directCodingJavaScriptPropertyError(
			input, body, target, source, computedKeys, form,
			question, cause,
		)
	}
	return nil
}

func directCodingJavaScriptPropertyError(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	computedKeys map[string]string,
	form javaScriptPropertyChoiceForm,
	question string,
	cause error,
) error {
	choices, err := directCodingJavaScriptPropertyChoices(
		input, body, node, source, computedKeys, form,
	)
	if err != nil {
		return fmt.Errorf("enumerate exact JavaScript property correction: %w", err)
	}
	return directCodingIdentifierNodeError(node, question, choices, cause)
}

func javaScriptNodeText(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func javaScriptExactPropertyCorrectionTarget(
	node *treesitter.Node,
	form javaScriptPropertyChoiceForm,
) (*treesitter.Node, javaScriptPropertyChoiceForm) {
	if node == nil {
		return nil, form
	}
	switch node.Kind() {
	case "computed_property_name":
		if node.NamedChildCount() != 1 {
			return nil, form
		}
		return javaScriptExactPropertyCorrectionTarget(
			node.NamedChild(0), javaScriptSubscriptPropertyChoice,
		)
	case "parenthesized_expression":
		if node.NamedChildCount() != 1 {
			return nil, form
		}
		return javaScriptExactPropertyCorrectionTarget(node.NamedChild(0), form)
	case "identifier", "property_identifier", "private_property_identifier",
		"shorthand_property_identifier_pattern", "string", "template_string":
		return node, form
	default:
		return nil, form
	}
}
