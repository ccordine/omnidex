package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateDirectCodingBrowserRuntimeDOMAuthority(
	root *treesitter.Node,
	source []byte,
) error {
	if root == nil {
		return fmt.Errorf("browser runtime DOM authority requires one syntax root")
	}
	eventBindings, err := collectDirectCodingBrowserEventBindings(root, source)
	if err != nil {
		return err
	}
	runtimeBindings, err := collectDirectCodingBrowserRuntimeBindings(root, source)
	if err != nil {
		return err
	}
	regExpAliases := collectDirectCodingBrowserRuntimeRegExpAliases(root, source, runtimeBindings)
	authoritativeValues := collectDirectCodingBrowserRuntimeAuthoritativeValues(
		root, source, runtimeBindings,
	)
	localProperties := collectDirectCodingBrowserRuntimeLocalProperties(root, source, runtimeBindings)
	numericIndices, err := collectDirectCodingBrowserNumericIndexBindings(root)
	if err != nil {
		return err
	}
	nodes := 0
	var inspect func(*treesitter.Node) error
	inspect = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		nodes++
		if nodes > directCodingBrowserPublicSurfaceMaxNodes {
			return fmt.Errorf("browser public surface exceeds %d syntax nodes", directCodingBrowserPublicSurfaceMaxNodes)
		}
		if directCodingBrowserRuntimeIdentifierIsEscaped(node, source) {
			return fmt.Errorf("browser public surface rejects escaped runtime identifier")
		}
		if name, reference := directCodingBrowserRuntimeReferenceName(source, node); reference {
			bound := runtimeBindings.binds(name, node)
			if _, forbidden := directCodingBrowserForbiddenRuntimeHostIdentifiers[name]; forbidden && !bound {
				return fmt.Errorf("browser public surface rejects runtime host authority identifier %s", name)
			}
			if !bound && !directCodingBrowserRuntimeGlobalPermitted(name) &&
				!directCodingBrowserRuntimeReferenceIsSyntax(node) {
				return fmt.Errorf("browser public surface rejects undeclared runtime identifier %s", name)
			}
			if !bound && directCodingBrowserRuntimeGlobalPermitted(name) &&
				directCodingBrowserRuntimeGlobalReferenceEscapes(node, name) {
				return fmt.Errorf("browser public surface rejects runtime global value escape %s", name)
			}
			if node.Kind() == "identifier" && eventBindings.reference(node, source) &&
				!directCodingBrowserEventReferenceIsPropertyObject(node) {
				return fmt.Errorf("browser public surface rejects runtime event authority alias or escape %s", name)
			}
		}
		if name, resolved, property := directCodingBrowserRuntimeProperty(source, node); property {
			if resolved {
				if directCodingBrowserRuntimeNondeterministicProperty(
					node, name, source, runtimeBindings, localProperties,
				) {
					return fmt.Errorf(
						"browser public surface rejects nondeterministic runtime property %s",
						name,
					)
				}
				if directCodingBrowserRuntimeReflectionPropertyForbidden(name) {
					return fmt.Errorf(
						"browser public surface rejects runtime reflection property %s", name,
					)
				}
				if regExpAliases.rejectsProperty(node, name, source, runtimeBindings) {
					return fmt.Errorf(
						"browser public surface rejects RegExp realm-global property %s", name,
					)
				}
			}
			if directCodingBrowserRuntimeGlobalPropertyEscapes(
				node, name, resolved, source, runtimeBindings,
			) {
				return fmt.Errorf("browser public surface rejects runtime global property value escape")
			}
			object := node.ChildByFieldName("object")
			if !resolved && directCodingBrowserExpressionContainsEventRoot(
				object, source, eventBindings,
			) {
				return fmt.Errorf("browser public surface rejects unresolved computed event property authority")
			}
			if directCodingBrowserExpressionIsEventRoot(object, source, eventBindings) {
				if !resolved {
					return fmt.Errorf("browser public surface rejects unresolved computed event property authority")
				}
				if name != "target" && name != "currentTarget" {
					return fmt.Errorf(
						"browser public surface rejects runtime event property %s outside target or currentTarget",
						name,
					)
				}
				if err := directCodingBrowserValidateEventTargetRead(node, source); err != nil {
					return err
				}
			}
			if directCodingBrowserExpressionIsEventTarget(object, source, eventBindings) {
				if !resolved || (name != "value" && name != "checked") {
					return fmt.Errorf("browser public surface rejects runtime DOM event-target property %s", name)
				}
			}
			if err := validateDirectCodingBrowserComputedProperty(
				node, source, runtimeBindings, numericIndices,
			); err != nil {
				return err
			}
		}
		switch node.Kind() {
		case "pair_pattern", "shorthand_property_identifier_pattern":
			if err := validateDirectCodingBrowserRuntimePatternAuthority(
				node, source, runtimeBindings, localProperties, eventBindings, regExpAliases,
			); err != nil {
				return err
			}
		case "assignment_expression", "augmented_assignment_expression":
			if err := directCodingBrowserValidateRuntimeDOMMutation(
				node.ChildByFieldName("left"), source, runtimeBindings, eventBindings,
				authoritativeValues,
			); err != nil {
				return err
			}
		case "update_expression":
			target := node.ChildByFieldName("argument")
			if target == nil && node.NamedChildCount() == 1 {
				target = node.NamedChild(0)
			}
			if err := directCodingBrowserValidateRuntimeDOMMutation(
				target, source, runtimeBindings, eventBindings, authoritativeValues,
			); err != nil {
				return err
			}
		case "unary_expression":
			if strings.HasPrefix(strings.TrimSpace(directCodingBrowserRuntimeNodeText(source, node)), "delete ") {
				target := node.ChildByFieldName("argument")
				if target == nil && node.NamedChildCount() == 1 {
					target = node.NamedChild(0)
				}
				if err := directCodingBrowserValidateRuntimeDOMMutation(
					target, source, runtimeBindings, eventBindings, authoritativeValues,
				); err != nil {
					return err
				}
			}
		case "for_in_statement", "for_of_statement":
			if err := directCodingBrowserValidateRuntimeDOMMutation(
				node.ChildByFieldName("left"), source, runtimeBindings, eventBindings,
				authoritativeValues,
			); err != nil {
				return err
			}
		case "meta_property":
			return fmt.Errorf("browser public surface rejects runtime host metadata authority")
		case "call_expression":
			if err := directCodingBrowserValidateAuthoritativeMutationCall(
				node, source, runtimeBindings, authoritativeValues,
			); err != nil {
				return err
			}
			callable := node.ChildByFieldName("function")
			if directCodingBrowserRuntimeNodeText(source, callable) == "import" {
				return fmt.Errorf("browser public surface rejects dynamic import authority")
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := inspect(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	return inspect(root)
}

func directCodingBrowserValidateEventTargetRead(node *treesitter.Node, source []byte) error {
	current := directCodingBrowserRuntimeOuterTransparentExpression(node)
	parent := current.Parent()
	if parent != nil {
		object := parent.ChildByFieldName("object")
		if object != nil && object.Id() == current.Id() {
			name, resolved, property := directCodingBrowserRuntimeProperty(source, parent)
			if property && resolved && (name == "value" || name == "checked") {
				return nil
			}
		}
	}
	name, _, _ := directCodingBrowserRuntimeProperty(source, node)
	return fmt.Errorf(
		"browser public surface rejects runtime DOM event-target authority %s outside read-only value or checked access",
		name,
	)
}

func directCodingBrowserExpressionIsEventRoot(
	node *treesitter.Node,
	source []byte,
	eventBindings directCodingBrowserEventBindings,
) bool {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return false
	}
	if node.Kind() == "identifier" {
		return eventBindings.reference(node, source)
	}
	return false
}

func directCodingBrowserExpressionIsEventTarget(
	node *treesitter.Node,
	source []byte,
	eventBindings directCodingBrowserEventBindings,
) bool {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	name, resolved, property := directCodingBrowserRuntimeProperty(source, node)
	return property && resolved && (name == "target" || name == "currentTarget") &&
		directCodingBrowserExpressionIsEventRoot(node.ChildByFieldName("object"), source, eventBindings)
}

func directCodingBrowserUnwrapRuntimeExpression(node *treesitter.Node) *treesitter.Node {
	for node != nil {
		switch node.Kind() {
		case "parenthesized_expression", "non_null_expression", "as_expression",
			"type_assertion", "satisfies_expression":
			expression := node.ChildByFieldName("expression")
			if expression == nil && node.NamedChildCount() > 0 {
				expression = node.NamedChild(0)
			}
			node = expression
		default:
			return node
		}
	}
	return nil
}

func directCodingBrowserRuntimeOuterTransparentExpression(node *treesitter.Node) *treesitter.Node {
	for node != nil {
		parent := node.Parent()
		if parent == nil || !directCodingBrowserRuntimeTransparentExpression(parent, node) {
			return node
		}
		node = parent
	}
	return nil
}

func directCodingBrowserRuntimeTransparentExpression(parent, child *treesitter.Node) bool {
	if parent == nil || child == nil {
		return false
	}
	switch parent.Kind() {
	case "parenthesized_expression", "non_null_expression", "as_expression",
		"type_assertion", "satisfies_expression":
		for index := uint(0); index < parent.NamedChildCount(); index++ {
			if candidate := parent.NamedChild(index); candidate != nil && candidate.Id() == child.Id() {
				return true
			}
		}
	}
	return false
}
