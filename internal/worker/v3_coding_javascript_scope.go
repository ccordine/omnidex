package worker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javascriptgrammar "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

var javaScriptDeclaredAPIPattern = regexp.MustCompile(
	`(?:^|[;\n]\s*)(?:export\s+)?(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`,
)

var javaScriptDeclaredStringConstantPattern = regexp.MustCompile(
	`(?:^|[;\n]\s*)(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:"([A-Za-z_$][A-Za-z0-9_$]*)"|'([A-Za-z_$][A-Za-z0-9_$]*)')\s*;?`,
)

func validateDirectCodingJavaScriptFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := assemblyline.ValidateJavaScriptFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	allowed := map[string]struct{}{
		"undefined": {}, "String": {}, "Number": {}, "Boolean": {}, "BigInt": {},
		"Array": {}, "Map": {}, "Set": {}, "JSON": {},
		"parseInt": {}, "parseFloat": {}, "isNaN": {}, "Infinity": {}, "NaN": {},
	}
	computedKeys := make(map[string]string)
	replaceableExternal := directCodingExplicitIdentifierAuthorities(
		input, javaScriptIdentifier,
	)
	for _, authority := range append(
		append([]string(nil), input.Capabilities...), input.PermittedSymbols...,
	) {
		trimmed := strings.TrimSpace(authority)
		if javaScriptIdentifier(trimmed) {
			allowed[trimmed] = struct{}{}
		}
		for _, match := range javaScriptDeclaredAPIPattern.FindAllStringSubmatch(trimmed, -1) {
			allowed[match[1]] = struct{}{}
		}
		for _, match := range javaScriptDeclaredStringConstantPattern.FindAllStringSubmatch(trimmed, -1) {
			value := match[2]
			if value == "" {
				value = match[3]
			}
			if !javaScriptSensitiveProperty(value) {
				computedKeys[match[1]] = value
			}
		}
	}
	if err := validateJavaScriptFreeIdentifiers(
		input, candidate, []byte(validated), allowed, replaceableExternal, computedKeys,
	); err != nil {
		return "", directCodingSourceBodyError(input, candidate, validated, err)
	}
	return validated, nil
}

func validateJavaScriptFreeIdentifiers(
	input assemblyline.FragmentGenerationInput,
	body string,
	source []byte,
	allowed map[string]struct{},
	replaceableExternal map[string]struct{},
	computedKeys map[string]string,
) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javascriptgrammar.Language())); err != nil {
		return err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("JavaScript scope parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("JavaScript scope parser rejected the declaration")
	}
	forbidden := map[string]struct{}{
		"process": {}, "require": {}, "eval": {}, "fetch": {}, "WebSocket": {},
		"globalThis": {}, "window": {}, "document": {}, "Function": {}, "Worker": {},
	}
	bindings, err := collectJavaScriptLexicalBindings(root, source, allowed, forbidden)
	if err != nil {
		return err
	}
	var inspect func(*treesitter.Node) error
	inspect = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		if err := directCodingJavaScriptClosedAuthorityError(
			input, body, node, root, source, bindings, computedKeys,
		); err != nil {
			return err
		}
		if node.Kind() == "identifier" {
			name := string(source[node.StartByte():node.EndByte()])
			bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
			failedStart := int(node.StartByte()) - bodyStart
			failedEnd := int(node.EndByte()) - bodyStart
			if _, denied := forbidden[name]; denied {
				replacements, replacementErr := directCodingJavaScriptIdentifierChoices(
					input, body, failedStart, failedEnd,
					name, node, source, bindings, replaceableExternal,
				)
				if replacementErr != nil {
					return replacementErr
				}
				return directCodingIdentifierNodeError(
					node,
					"Which available value has the meaning required at this unavailable reference?",
					replacements,
					fmt.Errorf("JavaScript fragment uses forbidden identifier %s", name),
				)
			}
			if !bindings.declaration(node) &&
				!javaScriptNonReferenceIdentifier(node) {
				_, externallyAllowed := allowed[name]
				if !externallyAllowed && !bindings.referenceAllowed(name, node) {
					replacements, replacementErr := directCodingJavaScriptIdentifierChoices(
						input, body, failedStart, failedEnd,
						name, node, source, bindings, replaceableExternal,
					)
					if replacementErr != nil {
						return replacementErr
					}
					return directCodingIdentifierNodeError(
						node,
						"Which available value has the meaning required at this unresolved reference?",
						replacements,
						fmt.Errorf("JavaScript fragment references undeclared direct symbol %s", name),
					)
				}
			}
		}
		for index := uint(0); index < node.ChildCount(); index++ {
			if err := inspect(node.Child(index)); err != nil {
				return err
			}
		}
		return nil
	}
	return inspect(root)
}

func javaScriptSensitiveProperty(name string) bool {
	switch strings.TrimSpace(name) {
	case "constructor", "prototype", "__proto__",
		"__defineGetter__", "__defineSetter__", "__lookupGetter__", "__lookupSetter__",
		"caller", "callee", "getPrototypeOf", "setPrototypeOf",
		"getOwnPropertyDescriptor", "getOwnPropertyDescriptors", "getOwnPropertyNames",
		"getOwnPropertySymbols", "defineProperty", "defineProperties":
		return true
	default:
		return false
	}
}

func javaScriptNumericSubscript(node *treesitter.Node) bool {
	return node != nil && node.Kind() == "number"
}

func javaScriptCodeOwnedComputedKey(
	source []byte,
	node *treesitter.Node,
	computedKeys map[string]string,
) bool {
	if node == nil || node.Kind() != "identifier" {
		return false
	}
	_, exists := computedKeys[string(source[node.StartByte():node.EndByte()])]
	return exists
}

func javaScriptPatternProperty(
	source []byte,
	key *treesitter.Node,
	computedKeys map[string]string,
) (string, bool, bool) {
	if key == nil {
		return "", false, false
	}
	switch key.Kind() {
	case "property_identifier", "private_property_identifier":
		return string(source[key.StartByte():key.EndByte()]), true, true
	case "number":
		return "", false, true
	case "string":
		value, resolved := javaScriptStaticPropertyName(source, key)
		return value, resolved, resolved
	case "computed_property_name":
		if key.NamedChildCount() != 1 {
			return "", false, false
		}
		expression := key.NamedChild(0)
		if value, resolved := javaScriptStaticPropertyName(source, expression); resolved {
			return value, true, true
		}
		if expression.Kind() == "identifier" {
			value, exists := computedKeys[string(source[expression.StartByte():expression.EndByte()])]
			return value, exists, exists
		}
	}
	return "", false, false
}

func javaScriptNonReferenceIdentifier(node *treesitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	if parent.Kind() == "member_expression" {
		property := parent.ChildByFieldName("property")
		return property != nil && property.Id() == node.Id()
	}
	return false
}

func javaScriptIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if index == 0 && char != '_' && char != '$' &&
			(char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
		if index > 0 && char != '_' && char != '$' &&
			(char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') {
			return false
		}
	}
	return true
}
