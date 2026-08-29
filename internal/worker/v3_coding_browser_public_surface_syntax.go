package worker

import (
	"fmt"
	"strings"
	"unicode"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserPublicSemanticAttributes = map[string]struct{}{
	"accessKey": {}, "alt": {}, "aria-disabled": {}, "aria-hidden": {}, "aria-label": {},
	"aria-labelledby": {}, "aria-readonly": {}, "children": {}, "className": {},
	"contentEditable": {}, "controls": {}, "dangerouslySetInnerHTML": {}, "draggable": {},
	"disabled": {}, "display": {}, "hidden": {}, "htmlFor": {}, "href": {}, "id": {},
	"inert": {}, "list": {}, "multiple": {}, "open": {}, "placeholder": {},
	"popover": {}, "readOnly": {}, "ref": {}, "role": {}, "size": {}, "style": {}, "tabIndex": {},
	"title": {}, "type": {}, "visibility": {}, "autoFocus": {},
}

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
	attributes := make(map[string]directCodingBrowserJSXAttribute)
	eventAttributes := make([]string, 0, 1)
	for index := uint(0); index < element.NamedChildCount(); index++ {
		child := element.NamedChild(index)
		if child == nil || child == name {
			continue
		}
		if child.Kind() == "jsx_expression" {
			return "", nil, fmt.Errorf("browser public surface rejects spread attributes")
		}
		if child.Kind() != "jsx_attribute" || child.NamedChildCount() == 0 {
			continue
		}
		attributeNameNode := child.NamedChild(0)
		if attributeNameNode == nil || attributeNameNode.Kind() != "property_identifier" {
			return "", nil, fmt.Errorf("browser public surface rejects namespaced attributes")
		}
		attributeName := extractor.nodeText(attributeNameNode)
		if directCodingBrowserEventHandlerAttribute(attributeName) {
			eventAttributes = append(eventAttributes, attributeName)
		}
		if _, relevant := directCodingBrowserPublicSemanticAttributes[attributeName]; !relevant {
			continue
		}
		if attributes[attributeName].present {
			return "", nil, fmt.Errorf("browser public surface rejects duplicate public attribute %s", attributeName)
		}
		attribute := directCodingBrowserJSXAttribute{present: true, boolean: child.NamedChildCount() == 1}
		if !attribute.boolean {
			value := child.NamedChild(1)
			if value == nil || value.Kind() != "string" {
				return "", nil, fmt.Errorf("browser public surface rejects dynamic public attribute %s", attributeName)
			}
			literal, err := extractor.quotedAttribute(value)
			if err != nil {
				return "", nil, err
			}
			attribute.literal = literal
		}
		attributes[attributeName] = attribute
	}
	for _, unsupported := range []string{
		"accessKey", "alt", "aria-labelledby", "autoFocus", "children",
		"contentEditable", "dangerouslySetInnerHTML", "display", "draggable",
		"open", "popover", "ref", "role", "style", "tabIndex", "title", "visibility",
	} {
		if attributes[unsupported].present {
			return "", nil, fmt.Errorf("browser public surface rejects unsupported public attribute %s", unsupported)
		}
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
	for _, requiredLiteral := range []string{
		"aria-disabled", "aria-hidden", "aria-label", "aria-readonly", "className",
		"htmlFor", "href", "id", "list", "placeholder", "size", "type",
	} {
		if attributes[requiredLiteral].present && attributes[requiredLiteral].boolean {
			return "", nil, fmt.Errorf("browser public surface attribute %s requires an exact literal", requiredLiteral)
		}
	}
	if attributes["htmlFor"].present && tag != "label" {
		return "", nil, fmt.Errorf("browser public surface rejects htmlFor outside a label")
	}
	for _, key := range []string{"aria-label", "className", "placeholder"} {
		attribute := attributes[key]
		if attribute.present {
			attribute.literal = normalizeDirectCodingBrowserPublicLiteral(attribute.literal)
			if key == "aria-label" && attribute.literal == "" {
				return "", nil, fmt.Errorf("browser public surface aria-label is empty")
			}
			attributes[key] = attribute
		}
	}
	for _, key := range []string{"id", "htmlFor"} {
		attribute := attributes[key]
		if attribute.present && (attribute.literal == "" || strings.IndexFunc(attribute.literal, unicode.IsSpace) >= 0) {
			return "", nil, fmt.Errorf("browser public surface attribute %s is not an exact identifier", key)
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
