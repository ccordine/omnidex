package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func directCodingBrowserValidateRuntimeDOMMutation(
	target *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	eventBindings directCodingBrowserEventBindings,
	authoritativeValues directCodingBrowserRuntimeAuthoritativeValues,
) error {
	target = directCodingBrowserUnwrapRuntimeExpression(target)
	if target == nil {
		return nil
	}
	switch target.Kind() {
	case "identifier", "shorthand_property_identifier_pattern":
		if authoritativeValues.reference(target, source, bindings) {
			return fmt.Errorf("browser public surface rejects authoritative state mutation")
		}
		name := directCodingBrowserRuntimeNodeText(source, target)
		if directCodingBrowserRuntimeGlobalPermitted(name) && !bindings.binds(name, target) {
			return fmt.Errorf("browser public surface rejects runtime global mutation %s", name)
		}
		return nil
	case "member_expression", "subscript_expression":
		name, resolved, _ := directCodingBrowserRuntimeProperty(source, target)
		if authoritativeValues.expressionMayBeAuthoritative(
			target.ChildByFieldName("object"), source, bindings,
		) {
			return fmt.Errorf("browser public surface rejects authoritative state mutation")
		}
		if resolved && directCodingBrowserExpressionIsEventTarget(
			target.ChildByFieldName("object"), source, eventBindings,
		) && (name == "value" || name == "checked") {
			return fmt.Errorf(
				"browser public surface rejects runtime DOM event-target mutation property %s", name,
			)
		}
		if root, global := directCodingBrowserRuntimePermittedGlobalRoot(
			target.ChildByFieldName("object"), source, bindings,
		); global {
			if resolved && directCodingBrowserRuntimeNondeterministicPropertyName(name) {
				return fmt.Errorf(
					"browser public surface rejects nondeterministic runtime property %s", name,
				)
			}
			if resolved && root == "RegExp" && directCodingBrowserRegExpLegacyStaticProperty(name) {
				return fmt.Errorf(
					"browser public surface rejects RegExp realm-global property %s", name,
				)
			}
			return fmt.Errorf("browser public surface rejects runtime global mutation %s", root)
		}
		return nil
	case "array_pattern", "object_pattern":
		for index := uint(0); index < target.NamedChildCount(); index++ {
			if err := directCodingBrowserValidateRuntimeDOMMutation(
				target.NamedChild(index), source, bindings, eventBindings, authoritativeValues,
			); err != nil {
				return err
			}
		}
		return nil
	case "pair_pattern":
		return directCodingBrowserValidateRuntimeDOMMutation(
			target.ChildByFieldName("value"), source, bindings, eventBindings,
			authoritativeValues,
		)
	case "assignment_pattern", "object_assignment_pattern":
		left := target.ChildByFieldName("left")
		if left == nil && target.NamedChildCount() > 0 {
			left = target.NamedChild(0)
		}
		return directCodingBrowserValidateRuntimeDOMMutation(
			left, source, bindings, eventBindings, authoritativeValues,
		)
	case "rest_pattern":
		argument := target.ChildByFieldName("argument")
		if argument == nil && target.NamedChildCount() > 0 {
			argument = target.NamedChild(0)
		}
		return directCodingBrowserValidateRuntimeDOMMutation(
			argument, source, bindings, eventBindings, authoritativeValues,
		)
	default:
		if strings.HasSuffix(target.Kind(), "_pattern") {
			for index := uint(0); index < target.NamedChildCount(); index++ {
				if err := directCodingBrowserValidateRuntimeDOMMutation(
					target.NamedChild(index), source, bindings, eventBindings,
					authoritativeValues,
				); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func directCodingBrowserValidateAuthoritativeMutationCall(
	call *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	authoritativeValues directCodingBrowserRuntimeAuthoritativeValues,
) error {
	callee := directCodingBrowserUnwrapRuntimeExpression(call.ChildByFieldName("function"))
	name, resolved, property := directCodingBrowserRuntimeProperty(source, callee)
	if !property || !resolved {
		return nil
	}
	receiver := callee.ChildByFieldName("object")
	if directCodingBrowserRuntimeAuthoritativeMutator(name) &&
		authoritativeValues.expressionMayBeAuthoritative(receiver, source, bindings) {
		return fmt.Errorf("browser public surface rejects authoritative state mutation")
	}
	root, global := directCodingBrowserRuntimePermittedGlobalRoot(receiver, source, bindings)
	if !global || root != "Object" || name != "assign" {
		return nil
	}
	arguments := call.ChildByFieldName("arguments")
	if arguments != nil && arguments.NamedChildCount() > 0 &&
		authoritativeValues.expressionMayBeAuthoritative(
			arguments.NamedChild(0), source, bindings,
		) {
		return fmt.Errorf("browser public surface rejects authoritative state mutation")
	}
	return nil
}

func directCodingBrowserRuntimeAuthoritativeMutator(name string) bool {
	switch name {
	case "copyWithin", "fill", "pop", "push", "reverse", "shift", "sort", "splice", "unshift":
		return true
	default:
		return false
	}
}
