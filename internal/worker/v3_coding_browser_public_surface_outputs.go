package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (extractor *directCodingBrowserPublicSurfaceExtractor) addOutput(
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
	element *treesitter.Node,
) error {
	if tag != "output" {
		return nil
	}
	name := attributes["aria-label"]
	if !name.present || name.boolean || name.literal == "" {
		return fmt.Errorf(
			"browser public output requires one exact literal aria-label",
		)
	}
	if err := validateDirectCodingBrowserPublicLiteral(name.literal); err != nil {
		return fmt.Errorf("browser public output accessible name: %w", err)
	}
	dynamic, literal, err := extractor.directOutputContent(element)
	if err != nil {
		return err
	}
	if !dynamic {
		return fmt.Errorf(
			"browser public output requires direct dynamic-only content",
		)
	}
	if literal != "" {
		return fmt.Errorf(
			"browser public output rejects mixed literal and dynamic content",
		)
	}
	if len(extractor.outputs) >= directCodingBrowserPublicSurfaceMaxOutputs {
		return fmt.Errorf(
			"browser public surface exceeds %d outputs",
			directCodingBrowserPublicSurfaceMaxOutputs,
		)
	}
	if _, duplicate := extractor.seenOutputs[name.literal]; duplicate {
		return fmt.Errorf(
			"browser public outputs repeat accessible name %q",
			name.literal,
		)
	}
	extractor.seenOutputs[name.literal] = struct{}{}
	extractor.outputs = append(extractor.outputs, directCodingBrowserPublicOutput{
		AccessibleName: name.literal,
	})
	return nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) directOutputContent(
	element *treesitter.Node,
) (bool, string, error) {
	var literal strings.Builder
	dynamic := false
	for index := uint(0); index < element.NamedChildCount(); index++ {
		child := element.NamedChild(index)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "jsx_opening_element", "jsx_closing_element":
			continue
		case "jsx_text", "html_character_reference":
			literal.WriteString(extractor.nodeText(child))
		case "jsx_expression":
			if treeSitterNodeContainsKind(
				child, "jsx_element", "jsx_self_closing_element",
			) {
				return false, "", fmt.Errorf(
					"browser public output requires direct dynamic-only content",
				)
			}
			if !directCodingBrowserJSXExpressionHasContent(child) {
				continue
			}
			if !extractor.outputExpressionIsRuntimeDerived(child) {
				return false, "", fmt.Errorf(
					"browser public output expression is not runtime-derived",
				)
			}
			dynamic = true
		case "jsx_element", "jsx_self_closing_element":
			return false, "", fmt.Errorf(
				"browser public output requires direct dynamic-only content",
			)
		}
	}
	return dynamic, normalizeDirectCodingBrowserPublicLiteral(literal.String()), nil
}
