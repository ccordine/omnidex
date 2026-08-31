package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementVisibility(
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
) (bool, error) {
	if directCodingBrowserNonPublicElement(tag) {
		return true, nil
	}
	if attributes["hidden"].present || attributes["inert"].present {
		return true, nil
	}
	if ariaHidden := attributes["aria-hidden"]; ariaHidden.present {
		value, err := directCodingBrowserExactARIABoolean("aria-hidden", ariaHidden)
		if err != nil {
			return false, err
		}
		if value {
			return true, nil
		}
	}
	return false, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementNodeHidden(
	element *treesitter.Node,
) (bool, error) {
	if element == nil || element.Kind() != "jsx_element" {
		return false, nil
	}
	tag, attributes, err := extractor.elementHeader(element.ChildByFieldName("open_tag"))
	if err != nil {
		return false, err
	}
	return extractor.elementVisibility(tag, attributes)
}

func directCodingBrowserControlUnavailable(
	attributes map[string]directCodingBrowserJSXAttribute,
) (bool, string, error) {
	for _, attributeName := range []string{"disabled", "readOnly"} {
		if attributes[attributeName].present {
			return true, attributeName, nil
		}
	}
	for _, attributeName := range []string{"aria-disabled", "aria-readonly"} {
		attribute := attributes[attributeName]
		if !attribute.present {
			continue
		}
		value, err := directCodingBrowserExactARIABoolean(attributeName, attribute)
		if err != nil {
			return false, "", err
		}
		if value {
			return true, attributeName, nil
		}
	}
	return false, "", nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) rejectUnavailableControlAncestry(
	element *treesitter.Node,
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
) error {
	reason := ""
	if tag == "fieldset" && attributes["disabled"].present {
		reason = "disabled fieldset"
	}
	if ariaDisabled := attributes["aria-disabled"]; ariaDisabled.present {
		value, err := directCodingBrowserExactARIABoolean("aria-disabled", ariaDisabled)
		if err != nil {
			return err
		}
		if value {
			_, _, control, controlErr := directCodingBrowserIntrinsicControl(tag, attributes)
			if controlErr != nil {
				return controlErr
			}
			if !control {
				reason = "aria-disabled"
			}
		}
	}
	if reason == "" {
		return nil
	}
	containsControl, err := extractor.expressionContainsControl(element)
	if err != nil {
		return err
	}
	if containsControl {
		return fmt.Errorf("browser public surface rejects controls under %s ancestry", reason)
	}
	return nil
}

func directCodingBrowserExactARIABoolean(
	name string,
	attribute directCodingBrowserJSXAttribute,
) (bool, error) {
	if attribute.boolean {
		return false, fmt.Errorf("browser public surface attribute %s requires true or false", name)
	}
	switch strings.ToLower(strings.TrimSpace(attribute.literal)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("browser public surface attribute %s requires true or false", name)
	}
}

func directCodingBrowserNonPublicElement(tag string) bool {
	switch tag {
	case "script", "style", "template", "noscript":
		return true
	default:
		return false
	}
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) boundedConditionalJSXExpression(
	node *treesitter.Node,
) bool {
	if node == nil || node.NamedChildCount() != 1 {
		return false
	}
	expression := directCodingBrowserUnwrapParenthesized(node.NamedChild(0))
	if expression == nil || expression.Kind() != "binary_expression" {
		return false
	}
	left := expression.ChildByFieldName("left")
	right := expression.ChildByFieldName("right")
	if left == nil || right == nil || strings.TrimSpace(
		string(extractor.source[left.EndByte():right.StartByte()]),
	) != "&&" || treeSitterNodeContainsKind(left, "jsx_element", "jsx_self_closing_element") {
		return false
	}
	render := directCodingBrowserUnwrapParenthesized(right)
	return render != nil && (render.Kind() == "jsx_element" ||
		render.Kind() == "jsx_self_closing_element")
}

func directCodingBrowserUnwrapParenthesized(node *treesitter.Node) *treesitter.Node {
	for node != nil && node.Kind() == "parenthesized_expression" {
		if node.NamedChildCount() != 1 {
			return nil
		}
		node = node.NamedChild(0)
	}
	return node
}
