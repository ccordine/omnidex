package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserForbiddenRuntimeHostIdentifiers = map[string]struct{}{
	// Global object and browsing-context authority.
	"document": {}, "window": {}, "globalThis": {}, "self": {},
	"frames": {}, "parent": {}, "top": {}, "opener": {},
	"navigator": {}, "location": {}, "history": {},

	// Network, cross-context, worker, and persistent-storage authority.
	"fetch": {}, "XMLHttpRequest": {}, "WebSocket": {},
	"EventSource": {}, "WebTransport": {}, "BroadcastChannel": {},
	"Worker": {}, "SharedWorker": {}, "importScripts": {},
	"localStorage": {}, "sessionStorage": {}, "caches": {}, "indexedDB": {},

	// Audio runtime authority.
	"AudioContext": {}, "OfflineAudioContext": {},
	"webkitAudioContext": {}, "webkitOfflineAudioContext": {},

	// Dynamic evaluation, reflection, and document-selection authority.
	"eval": {}, "Function": {}, "Proxy": {}, "Reflect": {},
	"getSelection": {},
}

var directCodingBrowserForbiddenRuntimeReflectionProperties = map[string]struct{}{
	"constructor": {}, "prototype": {}, "__proto__": {},
	"caller": {}, "callee": {}, "arguments": {},
	"defineProperty": {}, "defineProperties": {},
	"getPrototypeOf": {}, "setPrototypeOf": {},
	"getOwnPropertyDescriptor": {}, "getOwnPropertyDescriptors": {},
	"getOwnPropertyNames": {}, "getOwnPropertySymbols": {},
}

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
		if name, reference := directCodingBrowserRuntimeReferenceName(source, node); reference {
			if _, forbidden := directCodingBrowserForbiddenRuntimeHostIdentifiers[name]; forbidden && !runtimeBindings.binds(name, node) {
				return fmt.Errorf("browser public surface rejects runtime host authority identifier %s", name)
			}
			if node.Kind() == "identifier" && eventBindings.reference(node, source) &&
				!directCodingBrowserEventReferenceIsPropertyObject(node) {
				return fmt.Errorf("browser public surface rejects runtime event authority alias or escape %s", name)
			}
		}
		if name, resolved, property := directCodingBrowserRuntimeProperty(source, node); property {
			if resolved {
				if _, forbidden := directCodingBrowserForbiddenRuntimeReflectionProperties[name]; forbidden {
					return fmt.Errorf(
						"browser public surface rejects runtime reflection property %s", name,
					)
				}
			}
			object := node.ChildByFieldName("object")
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
		}
		switch node.Kind() {
		case "pair_pattern", "shorthand_property_identifier_pattern":
			if directCodingBrowserPatternAliasesEventTarget(node, source, eventBindings) {
				return fmt.Errorf("browser public surface rejects runtime DOM event-target destructuring")
			}
		case "assignment_expression", "augmented_assignment_expression":
			if err := directCodingBrowserValidateRuntimeDOMMutation(
				node.ChildByFieldName("left"), source, eventBindings,
			); err != nil {
				return err
			}
		case "update_expression":
			target := node.ChildByFieldName("argument")
			if target == nil && node.NamedChildCount() == 1 {
				target = node.NamedChild(0)
			}
			if err := directCodingBrowserValidateRuntimeDOMMutation(target, source, eventBindings); err != nil {
				return err
			}
		case "unary_expression":
			if strings.HasPrefix(strings.TrimSpace(directCodingBrowserRuntimeNodeText(source, node)), "delete ") {
				target := node.ChildByFieldName("argument")
				if target == nil && node.NamedChildCount() == 1 {
					target = node.NamedChild(0)
				}
				if err := directCodingBrowserValidateRuntimeDOMMutation(target, source, eventBindings); err != nil {
					return err
				}
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

func directCodingBrowserRuntimeReferenceName(
	source []byte,
	node *treesitter.Node,
) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind() {
	case "identifier", "shorthand_property_identifier":
		return directCodingBrowserRuntimeNodeText(source, node), true
	default:
		return "", false
	}
}

func directCodingBrowserRuntimeProperty(
	source []byte,
	node *treesitter.Node,
) (string, bool, bool) {
	if node == nil {
		return "", false, false
	}
	switch node.Kind() {
	case "member_expression":
		property := node.ChildByFieldName("property")
		if property == nil {
			return "", false, true
		}
		return directCodingBrowserRuntimeNodeText(source, property), true, true
	case "subscript_expression":
		name, resolved := javaScriptStaticPropertyName(source, node.ChildByFieldName("index"))
		return name, resolved, true
	default:
		return "", false, false
	}
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

func directCodingBrowserValidateRuntimeDOMMutation(
	target *treesitter.Node,
	source []byte,
	eventBindings directCodingBrowserEventBindings,
) error {
	target = directCodingBrowserUnwrapRuntimeExpression(target)
	name, resolved, property := directCodingBrowserRuntimeProperty(source, target)
	if !property || !resolved {
		return nil
	}
	if directCodingBrowserExpressionIsEventTarget(
		target.ChildByFieldName("object"), source, eventBindings,
	) && (name == "value" || name == "checked") {
		return fmt.Errorf("browser public surface rejects runtime DOM event-target mutation property %s", name)
	}
	return nil
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

func directCodingBrowserRuntimeNodeText(source []byte, node *treesitter.Node) string {
	if node == nil || node.EndByte() > uint(len(source)) {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}
