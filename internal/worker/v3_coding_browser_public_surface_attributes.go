package worker

import (
	"fmt"
	"strings"
	"unicode"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserExactAttributes = map[string]struct{}{
	"aria-disabled": {}, "aria-hidden": {}, "aria-label": {}, "aria-readonly": {},
	"className": {}, "htmlFor": {}, "id": {}, "list": {}, "placeholder": {},
	"size": {}, "type": {},
}

var directCodingBrowserBooleanAttributes = map[string]struct{}{
	"disabled": {}, "hidden": {}, "inert": {}, "multiple": {}, "readOnly": {},
}

func directCodingBrowserAllowedEventAttribute(name string) bool {
	switch name {
	case "onClick", "onChange", "onInput":
		return true
	default:
		return false
	}
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) elementAttributes(
	element *treesitter.Node,
	tagName *treesitter.Node,
	tag string,
) (map[string]directCodingBrowserJSXAttribute, []string, error) {
	attributes := make(map[string]directCodingBrowserJSXAttribute)
	events := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for index := uint(0); index < element.NamedChildCount(); index++ {
		child := element.NamedChild(index)
		if child == nil || child == tagName {
			continue
		}
		if child.Kind() == "jsx_expression" {
			return nil, nil, fmt.Errorf("browser public surface rejects spread attributes")
		}
		if child.Kind() != "jsx_attribute" || child.NamedChildCount() == 0 {
			continue
		}
		nameNode := child.NamedChild(0)
		if nameNode == nil || nameNode.Kind() != "property_identifier" {
			return nil, nil, fmt.Errorf("browser public surface rejects namespaced attributes")
		}
		name := extractor.nodeText(nameNode)
		if _, duplicate := seen[name]; duplicate {
			return nil, nil, fmt.Errorf(
				"browser public surface rejects duplicate attribute %s", name,
			)
		}
		seen[name] = struct{}{}
		if directCodingBrowserEventHandlerAttribute(name) {
			if !directCodingBrowserAllowedEventAttribute(name) {
				return nil, nil, fmt.Errorf(
					"browser public surface rejects unsupported event attribute %s", name,
				)
			}
			if child.NamedChildCount() != 2 || child.NamedChild(1).Kind() != "jsx_expression" {
				return nil, nil, fmt.Errorf(
					"browser public surface event attribute %s requires one bound expression", name,
				)
			}
			events = append(events, name)
			continue
		}
		if name == "value" || name == "checked" {
			attribute, err := extractor.valueAttribute(child, tag, name)
			if err != nil {
				return nil, nil, err
			}
			attributes[name] = attribute
			continue
		}
		if _, exact := directCodingBrowserExactAttributes[name]; exact {
			attribute, err := extractor.exactAttribute(child, name)
			if err != nil {
				return nil, nil, err
			}
			attributes[name] = attribute
			continue
		}
		if _, boolean := directCodingBrowserBooleanAttributes[name]; boolean {
			if child.NamedChildCount() != 1 {
				return nil, nil, fmt.Errorf(
					"browser public surface boolean attribute %s cannot carry a value", name,
				)
			}
			attributes[name] = directCodingBrowserJSXAttribute{present: true, boolean: true}
			continue
		}
		return nil, nil, fmt.Errorf(
			"browser public surface rejects unsupported attribute %s", name,
		)
	}
	if err := extractor.validateElementAttributes(tag, attributes); err != nil {
		return nil, nil, err
	}
	return attributes, events, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) exactAttribute(
	node *treesitter.Node,
	name string,
) (directCodingBrowserJSXAttribute, error) {
	if node.NamedChildCount() != 2 || node.NamedChild(1).Kind() != "string" {
		return directCodingBrowserJSXAttribute{}, fmt.Errorf(
			"browser public surface attribute %s requires an exact literal", name,
		)
	}
	literal, err := extractor.quotedAttribute(node.NamedChild(1))
	if err != nil {
		return directCodingBrowserJSXAttribute{}, err
	}
	return directCodingBrowserJSXAttribute{present: true, literal: literal}, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) valueAttribute(
	node *treesitter.Node,
	tag string,
	name string,
) (directCodingBrowserJSXAttribute, error) {
	validTag := name == "value" && (tag == "input" || tag == "select" ||
		tag == "textarea" || tag == "option") || name == "checked" && tag == "input"
	if !validTag || node.NamedChildCount() != 2 {
		return directCodingBrowserJSXAttribute{}, fmt.Errorf(
			"browser public surface rejects attribute %s on %s", name, tag,
		)
	}
	value := node.NamedChild(1)
	if value.Kind() != "string" && value.Kind() != "jsx_expression" {
		return directCodingBrowserJSXAttribute{}, fmt.Errorf(
			"browser public surface attribute %s requires a literal or bound expression", name,
		)
	}
	return directCodingBrowserJSXAttribute{present: true}, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) validateElementAttributes(
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
) error {
	compatible := map[string]map[string]struct{}{
		"checked":     {"input": {}},
		"disabled":    {"button": {}, "fieldset": {}, "input": {}, "select": {}, "textarea": {}},
		"htmlFor":     {"label": {}},
		"list":        {"input": {}},
		"multiple":    {"select": {}},
		"placeholder": {"input": {}, "textarea": {}},
		"readOnly":    {"input": {}, "textarea": {}},
		"size":        {"select": {}},
		"type":        {"button": {}, "input": {}},
		"value":       {"input": {}, "option": {}, "select": {}, "textarea": {}},
	}
	for name, tags := range compatible {
		if attributes[name].present {
			if _, allowed := tags[tag]; !allowed {
				return fmt.Errorf(
					"browser public surface rejects attribute %s on %s", name, tag,
				)
			}
		}
	}
	for _, key := range []string{"aria-label", "className", "placeholder"} {
		attribute := attributes[key]
		if attribute.present {
			attribute.literal = normalizeDirectCodingBrowserPublicLiteral(attribute.literal)
			if key == "aria-label" && attribute.literal == "" {
				return fmt.Errorf("browser public surface aria-label is empty")
			}
			attributes[key] = attribute
		}
	}
	for _, key := range []string{"id", "htmlFor"} {
		attribute := attributes[key]
		if attribute.present && (attribute.literal == "" ||
			strings.IndexFunc(attribute.literal, unicode.IsSpace) >= 0) {
			return fmt.Errorf(
				"browser public surface attribute %s is not an exact identifier", key,
			)
		}
	}
	return nil
}
