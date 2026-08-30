package worker

import (
	"fmt"
	"unicode/utf8"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func extractDirectCodingBrowserPublicInteractionSurface(
	source string,
) (directCodingBrowserPublicInteractionSurface, error) {
	return extractDirectCodingBrowserPublicInteractionSurfaceWithRuntimeCalls(source, nil)
}

func extractDirectCodingBrowserPublicInteractionSurfaceWithRuntimeCalls(
	source string,
	permittedRuntimeCalls []string,
) (directCodingBrowserPublicInteractionSurface, error) {
	if source == "" {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("browser public surface source is empty")
	}
	if len(source) > directCodingBrowserPublicSurfaceMaxSourceBytes {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("browser public surface source exceeds %d bytes", directCodingBrowserPublicSurfaceMaxSourceBytes)
	}
	if !utf8.ValidString(source) {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("browser public surface source is not valid UTF-8")
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(typescript.LanguageTSX())); err != nil {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("configure browser public surface TSX parser: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("browser public surface TSX parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return directCodingBrowserPublicInteractionSurface{}, fmt.Errorf("browser public surface source is not valid TSX")
	}
	renderRoot, err := directCodingBrowserPublicRenderRoot(root)
	if err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	outputFlow, err := newDirectCodingBrowserOutputDataflow(
		root, renderRoot, []byte(source),
	)
	if err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	extractor := directCodingBrowserPublicSurfaceExtractor{
		source:      []byte(source),
		outputFlow:  outputFlow,
		seenIDs:     make(map[string]struct{}),
		seenOutputs: make(map[string]struct{}),
	}
	if err := extractor.preflight(renderRoot); err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	if err := extractor.inspect(renderRoot, -1, false); err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	surface, err := extractor.finish()
	if err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	if err := validateDirectCodingBrowserRuntimeDOMAuthority(
		root, []byte(source), permittedRuntimeCalls,
	); err != nil {
		return directCodingBrowserPublicInteractionSurface{}, err
	}
	return surface, nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) preflight(root *treesitter.Node) error {
	nodes := 0
	var walk func(*treesitter.Node) error
	walk = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		nodes++
		if nodes > directCodingBrowserPublicSurfaceMaxNodes {
			return fmt.Errorf("browser public surface exceeds %d syntax nodes", directCodingBrowserPublicSurfaceMaxNodes)
		}
		if node.Kind() == "jsx_opening_element" || node.Kind() == "jsx_self_closing_element" {
			_, attributes, err := extractor.elementHeader(node)
			if err != nil {
				return err
			}
			if id := attributes["id"].literal; id != "" {
				if _, duplicate := extractor.seenIDs[id]; duplicate {
					return fmt.Errorf("browser public surface repeats public element id %q", id)
				}
				extractor.seenIDs[id] = struct{}{}
				extractor.ids = append(extractor.ids, id)
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := walk(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) inspect(
	node *treesitter.Node,
	label int,
	conditional bool,
) error {
	if node == nil {
		return nil
	}
	switch node.Kind() {
	case "jsx_element":
		return extractor.inspectElement(node, label, conditional)
	case "jsx_self_closing_element":
		return extractor.inspectSelfClosing(node, label)
	case "jsx_expression":
		if treeSitterNodeContainsKind(node, "jsx_element", "jsx_self_closing_element") {
			if !extractor.boundedConditionalJSXExpression(node) {
				return fmt.Errorf("browser public surface rejects unbounded JSX-producing expression")
			}
			containsControl, err := extractor.expressionContainsControl(node)
			if err != nil {
				return err
			}
			if containsControl {
				return fmt.Errorf("browser public surface rejects dynamic control cardinality")
			}
			for index := uint(0); index < node.NamedChildCount(); index++ {
				if err := extractor.inspect(node.NamedChild(index), label, true); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if err := extractor.inspect(node.NamedChild(index), label, conditional); err != nil {
			return err
		}
	}
	return nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) inspectElement(
	element *treesitter.Node,
	parentLabel int,
	conditional bool,
) error {
	opening := element.ChildByFieldName("open_tag")
	tag, attributes, err := extractor.elementHeader(opening)
	if err != nil {
		return err
	}
	hidden, err := extractor.elementVisibility(tag, attributes)
	if err != nil {
		return err
	}
	if hidden {
		containsControl, controlErr := extractor.expressionContainsControl(element)
		if controlErr != nil {
			return controlErr
		}
		if containsControl {
			return fmt.Errorf("browser public surface rejects inaccessible controls")
		}
		return nil
	}
	if err := extractor.rejectUnavailableControlAncestry(element, tag, attributes); err != nil {
		return err
	}
	label := parentLabel
	if tag == "label" {
		if parentLabel >= 0 {
			return fmt.Errorf("browser public surface rejects nested labels")
		}
		literal, dynamic, contentErr := extractor.elementLiteralContent(element)
		if contentErr != nil {
			return contentErr
		}
		if dynamic || literal == "" {
			return fmt.Errorf("browser public surface requires exact literal label text")
		}
		if err := validateDirectCodingBrowserPublicLiteral(literal); err != nil {
			return err
		}
		label = len(extractor.labels)
		extractor.labels = append(extractor.labels, directCodingBrowserPendingLabel{
			forID: attributes["htmlFor"].literal, literal: literal,
		})
	}
	if err := extractor.addControl(tag, attributes, element, label); err != nil {
		return err
	}
	if err := extractor.addOutput(tag, attributes, element); err != nil {
		return err
	}
	for index := uint(0); index < element.NamedChildCount(); index++ {
		child := element.NamedChild(index)
		if child == nil || child.Kind() == "jsx_opening_element" || child.Kind() == "jsx_closing_element" ||
			child.Kind() == "jsx_text" || child.Kind() == "html_character_reference" {
			continue
		}
		if err := extractor.inspect(child, label, conditional); err != nil {
			return err
		}
	}
	return nil
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) inspectSelfClosing(
	element *treesitter.Node,
	label int,
) error {
	tag, attributes, err := extractor.elementHeader(element)
	if err != nil {
		return err
	}
	if tag == "label" {
		return fmt.Errorf("browser public surface requires exact literal label text")
	}
	hidden, err := extractor.elementVisibility(tag, attributes)
	if err != nil {
		return err
	}
	if hidden {
		_, _, control, controlErr := directCodingBrowserIntrinsicControl(tag, attributes)
		if controlErr != nil {
			return controlErr
		}
		if control {
			return fmt.Errorf("browser public surface rejects inaccessible controls")
		}
		return nil
	}
	if tag == "output" {
		return fmt.Errorf(
			"browser public output requires direct dynamic-only content",
		)
	}
	return extractor.addControl(tag, attributes, nil, label)
}

func (extractor *directCodingBrowserPublicSurfaceExtractor) addControl(
	tag string,
	attributes map[string]directCodingBrowserJSXAttribute,
	element *treesitter.Node,
	label int,
) error {
	role, valueKind, control, err := directCodingBrowserIntrinsicControl(tag, attributes)
	if err != nil || !control {
		return err
	}
	if unavailable, reason, availabilityErr := directCodingBrowserControlUnavailable(attributes); availabilityErr != nil {
		return availabilityErr
	} else if unavailable {
		return fmt.Errorf("browser public surface rejects unavailable control state %s", reason)
	}
	if len(extractor.controls) >= directCodingBrowserPublicSurfaceMaxControls {
		return fmt.Errorf("browser public surface exceeds %d controls", directCodingBrowserPublicSurfaceMaxControls)
	}
	buttonText := ""
	if tag == "button" && element != nil {
		var dynamic bool
		var contentErr error
		buttonText, dynamic, contentErr = extractor.elementLiteralContent(element)
		if contentErr != nil {
			return contentErr
		}
		if dynamic || buttonText == "" {
			return fmt.Errorf("browser public surface requires exact literal button text")
		}
	}
	controlIndex := len(extractor.controls)
	if label >= 0 {
		extractor.labels[label].controls = append(extractor.labels[label].controls, controlIndex)
	}
	extractor.controls = append(extractor.controls, directCodingBrowserPendingControl{
		directCodingBrowserPublicControl: directCodingBrowserPublicControl{
			Role: role, AccessibleName: attributes["aria-label"].literal,
			PlaceholderHint: attributes["placeholder"].literal, ValueKind: valueKind,
		},
		id: attributes["id"].literal, label: label, buttonText: buttonText,
	})
	return nil
}
