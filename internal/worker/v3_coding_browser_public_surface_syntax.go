package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementHeader(
	element *treesitter.Node,
) (string, map[string]directCodingBrowserJSXAttribute, error) {
	if element == nil {
		return "", nil, fmt.Errorf("browser public surface element has no opening tag")
	}
	name := element.ChildByFieldName("name")
	if name == nil || name.Kind() != "identifier" {
		return "", nil, fmt.Errorf("browser public surface rejects custom components")
	}
	tag := extractor.nodeText(name)
	if !directCodingBrowserIntrinsicTag(tag) {
		return "", nil, fmt.Errorf("browser public surface rejects custom component %q", tag)
	}
	if directCodingBrowserNonPublicElement(tag) {
		return "", nil, fmt.Errorf(
			"browser public surface rejects embedded non-public element %q", tag,
		)
	}
	if tag == "dialog" {
		return "", nil, fmt.Errorf("browser public surface rejects visibility-bearing dialog element")
	}
	if !directCodingBrowserSupportedIntrinsicTag(tag) {
		return "", nil, fmt.Errorf(
			"browser public surface rejects unsupported intrinsic element %q", tag,
		)
	}
	attributes, eventAttributes, err := extractor.elementAttributes(element, name, tag)
	if err != nil {
		return "", nil, err
	}
	if len(eventAttributes) != 0 {
		_, _, control, controlErr := directCodingBrowserIntrinsicControl(tag, attributes)
		if controlErr != nil {
			return "", nil, controlErr
		}
		if !control {
			return "", nil, fmt.Errorf(
				"browser public surface rejects event attribute %s on unregistered intrinsic element %s",
				eventAttributes[0], tag,
			)
		}
	}
	return tag, attributes, nil
}

func directCodingBrowserEventHandlerAttribute(name string) bool {
	return len(name) > 2 && name[0] == 'o' && name[1] == 'n' &&
		name[2] >= 'A' && name[2] <= 'Z'
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) quotedAttribute(
	node *treesitter.Node,
) (string, error) {
	raw := extractor.nodeText(node)
	if len(raw) < 2 || (raw[0] != '\'' && raw[0] != '"') || raw[len(raw)-1] != raw[0] {
		return "", fmt.Errorf("browser public surface attribute is not exactly quoted")
	}
	return normalizeDirectCodingBrowserPublicLiteral(raw[1 : len(raw)-1]), nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) nodeText(node *treesitter.Node) string {
	return string(extractor.source[node.StartByte():node.EndByte()])
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementLiteralContent(
	element *treesitter.Node,
) (string, bool, error) {
	parts := make([]string, 0)
	dynamic := false
	var appendContent func(*treesitter.Node) error
	appendContent = func(owner *treesitter.Node) error {
		for index := uint(0); index < owner.NamedChildCount(); index++ {
			child := owner.NamedChild(index)
			if child == nil {
				continue
			}
			switch child.Kind() {
			case "jsx_text", "html_character_reference":
				parts = append(parts, extractor.nodeText(child))
			case "jsx_expression":
				if directCodingBrowserJSXExpressionHasContent(child) {
					dynamic = true
				}
			case "jsx_element":
				hidden, err := extractor.elementNodeHidden(child)
				if err != nil {
					return err
				}
				if !hidden && !extractor.elementIsControl(child) {
					if err := appendContent(child); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := appendContent(element); err != nil {
		return "", false, err
	}
	return normalizeDirectCodingBrowserPublicLiteral(strings.Join(parts, " ")), dynamic, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementIsControl(
	element *treesitter.Node,
) bool {
	opening := element.ChildByFieldName("open_tag")
	if opening == nil || opening.ChildByFieldName("name") == nil {
		return false
	}
	switch extractor.nodeText(opening.ChildByFieldName("name")) {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) expressionContainsControl(
	expression *treesitter.Node,
) (bool, error) {
	var inspect func(*treesitter.Node) (bool, error)
	inspect = func(node *treesitter.Node) (bool, error) {
		if node == nil {
			return false, nil
		}
		if node.Kind() == "jsx_opening_element" || node.Kind() == "jsx_self_closing_element" {
			tag, attributes, err := extractor.elementHeader(node)
			if err != nil {
				return false, err
			}
			_, _, control, err := directCodingBrowserIntrinsicControl(tag, attributes)
			return control, err
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			control, err := inspect(node.NamedChild(index))
			if err != nil || control {
				return control, err
			}
		}
		return false, nil
	}
	return inspect(expression)
}

func directCodingBrowserJSXExpressionHasContent(node *treesitter.Node) bool {
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child != nil && child.Kind() != "comment" {
			return true
		}
	}
	return false
}
