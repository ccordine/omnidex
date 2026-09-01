package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var directCodingBrowserPermittedRuntimeGlobalIdentifiers = map[string]struct{}{
	// ECMAScript value and collection primitives used by generated views.
	"undefined": {}, "NaN": {}, "Infinity": {},
	"String": {}, "Number": {}, "Boolean": {}, "BigInt": {},
	"Array": {}, "Object": {}, "Map": {}, "Set": {},
	"WeakMap": {}, "WeakSet": {}, "JSON": {}, "Math": {},
	"RegExp": {}, "Symbol": {}, "Error": {},
	"parseInt": {}, "parseFloat": {}, "isNaN": {}, "isFinite": {},
	"encodeURI": {}, "encodeURIComponent": {},
	"decodeURI": {}, "decodeURIComponent": {},

	// Exact adapter-owned React imports. Other free identifiers fail closed.
	"useCallback": {}, "useEffect": {}, "useMemo": {}, "useRef": {}, "useState": {},
}

var directCodingBrowserForbiddenRuntimeHostIdentifiers = map[string]struct{}{
	// Global object and browsing-context authority.
	"document": {}, "window": {}, "globalThis": {}, "self": {},
	"frames": {}, "parent": {}, "top": {}, "opener": {},
	"navigator": {}, "location": {}, "history": {}, "origin": {},
	"screen": {}, "visualViewport": {}, "customElements": {},

	// Network, cross-context, worker, and persistent-storage authority.
	"fetch": {}, "XMLHttpRequest": {}, "WebSocket": {},
	"EventSource": {}, "WebTransport": {}, "BroadcastChannel": {},
	"Worker": {}, "SharedWorker": {}, "ServiceWorker": {}, "importScripts": {},
	"MessageChannel": {}, "MessagePort": {}, "postMessage": {},
	"localStorage": {}, "sessionStorage": {}, "caches": {}, "indexedDB": {},
	"cookieStore": {},

	// DOM, resource, media, observer, file, and request constructors.
	"Audio": {}, "Image": {}, "AudioContext": {}, "OfflineAudioContext": {},
	"webkitAudioContext": {}, "webkitOfflineAudioContext": {},
	"DOMParser": {}, "XMLSerializer": {}, "MutationObserver": {},
	"ResizeObserver": {}, "IntersectionObserver": {}, "PerformanceObserver": {},
	"FileReader": {}, "File": {}, "Blob": {}, "FormData": {},
	"URL": {}, "URLSearchParams": {}, "Request": {}, "Response": {},
	"Headers": {}, "AbortController": {}, "AbortSignal": {},
	"Notification": {}, "MediaRecorder": {}, "MediaSource": {},
	"RTCPeerConnection": {}, "webkitRTCPeerConnection": {},
	"SpeechSynthesisUtterance": {}, "speechSynthesis": {},

	// Scheduling, browser UI, events, and nondeterministic host state.
	"setTimeout": {}, "clearTimeout": {}, "setInterval": {}, "clearInterval": {},
	"queueMicrotask": {}, "requestAnimationFrame": {}, "cancelAnimationFrame": {},
	"requestIdleCallback": {}, "cancelIdleCallback": {},
	"alert": {}, "confirm": {}, "prompt": {}, "open": {}, "close": {},
	"print": {}, "focus": {}, "blur": {}, "stop": {}, "matchMedia": {},
	"addEventListener": {}, "removeEventListener": {}, "dispatchEvent": {},
	"crypto": {}, "performance": {}, "Intl": {},

	// Dynamic evaluation, reflection, and document-selection authority.
	"eval": {}, "Function": {}, "Proxy": {}, "Reflect": {},
	"getSelection": {},
}

var directCodingBrowserForbiddenRuntimeReflectionProperties = map[string]struct{}{
	"constructor": {}, "prototype": {}, "__proto__": {},
	"__defineGetter__": {}, "__defineSetter__": {},
	"__lookupGetter__": {}, "__lookupSetter__": {},
	"call": {}, "apply": {}, "bind": {},
	"hasOwnProperty": {}, "propertyIsEnumerable": {},
	"caller": {}, "callee": {}, "arguments": {},
	"defineProperty": {}, "defineProperties": {},
	"getPrototypeOf": {}, "setPrototypeOf": {},
	"getOwnPropertyDescriptor": {}, "getOwnPropertyDescriptors": {},
	"getOwnPropertyNames": {}, "getOwnPropertySymbols": {},
}

func directCodingBrowserRuntimeGlobalPermitted(name string) bool {
	_, permitted := directCodingBrowserPermittedRuntimeGlobalIdentifiers[name]
	return permitted
}

func directCodingBrowserRuntimeIdentifierIsEscaped(
	node *treesitter.Node,
	source []byte,
) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier", "property_identifier", "private_property_identifier",
		"shorthand_property_identifier", "shorthand_property_identifier_pattern":
		return strings.Contains(directCodingBrowserRuntimeNodeText(source, node), `\`)
	default:
		return false
	}
}

func directCodingBrowserRuntimeNodeText(source []byte, node *treesitter.Node) string {
	if node == nil || node.EndByte() > uint(len(source)) {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func directCodingBrowserRuntimeReflectionPropertyForbidden(name string) bool {
	name = strings.TrimSpace(name)
	if _, forbidden := directCodingBrowserForbiddenRuntimeReflectionProperties[name]; forbidden {
		return true
	}
	return javaScriptSensitiveProperty(name)
}

func directCodingBrowserRuntimeNondeterministicProperty(
	node *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	localProperties directCodingBrowserRuntimeLocalProperties,
) bool {
	if node == nil {
		return false
	}
	object := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("object"))
	if localProperties.receiverOwns(object, name, source, bindings) {
		return false
	}
	return directCodingBrowserRuntimeNondeterministicPropertyName(name)
}

func directCodingBrowserRuntimeNondeterministicPatternProperty(
	node *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	localProperties directCodingBrowserRuntimeLocalProperties,
) bool {
	if directCodingBrowserRuntimeNondeterministicPropertyName(name) {
		return !localProperties.patternOwns(node, name, source, bindings)
	}
	return false
}

func directCodingBrowserRuntimeNondeterministicPropertyName(name string) bool {
	switch name {
	case "random", "stack", "captureStackTrace", "prepareStackTrace", "stackTraceLimit",
		"localeCompare", "toLocaleLowerCase", "toLocaleUpperCase",
		"toLocaleString", "toLocaleDateString", "toLocaleTimeString":
		return true
	default:
		return false
	}
}

func validateDirectCodingBrowserRuntimePatternAuthority(
	node *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	localProperties directCodingBrowserRuntimeLocalProperties,
	eventBindings directCodingBrowserEventBindings,
	regExpAliases directCodingBrowserRuntimeRegExpAliases,
) error {
	if err := validateDirectCodingBrowserRuntimePatternProperty(node, source); err != nil {
		return err
	}
	property := node
	if node != nil && node.Kind() == "pair_pattern" {
		property = node.ChildByFieldName("key")
	}
	name, resolved := directCodingBrowserRuntimePatternProperty(source, property)
	if resolved && regExpAliases.rejectsPattern(node, name, source, bindings) {
		return fmt.Errorf(
			"browser public surface rejects RegExp realm-global property %s", name,
		)
	}
	if resolved && directCodingBrowserRuntimeNondeterministicPatternProperty(
		node, name, source, bindings, localProperties,
	) {
		return fmt.Errorf(
			"browser public surface rejects nondeterministic runtime property %s", name,
		)
	}
	if directCodingBrowserPatternAliasesEventTarget(node, source, eventBindings) {
		return fmt.Errorf("browser public surface rejects runtime DOM event-target destructuring")
	}
	return nil
}

func directCodingBrowserRuntimeReferenceIsSyntax(node *treesitter.Node) bool {
	if node == nil || node.Parent() == nil {
		return false
	}
	parent := node.Parent()
	for current := parent; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "type_annotation", "type_arguments", "type_parameters",
			"implements_clause", "extends_type_clause":
			return true
		}
	}
	switch parent.Kind() {
	case "jsx_opening_element", "jsx_closing_element", "jsx_self_closing_element":
		return parent.NamedChildCount() > 0 && parent.NamedChild(0).Id() == node.Id()
	case "jsx_attribute":
		return parent.NamedChildCount() > 0 && parent.NamedChild(0).Id() == node.Id()
	default:
		return false
	}
}

func validateDirectCodingBrowserRuntimePatternProperty(
	node *treesitter.Node,
	source []byte,
) error {
	property := node
	if node != nil && node.Kind() == "pair_pattern" {
		property = node.ChildByFieldName("key")
	}
	name, resolved := directCodingBrowserRuntimePatternProperty(source, property)
	if !resolved {
		return fmt.Errorf("browser public surface rejects unresolved computed destructured property authority")
	}
	if directCodingBrowserRuntimeReflectionPropertyForbidden(name) {
		return fmt.Errorf("browser public surface rejects runtime reflection property %s", name)
	}
	return nil
}
