package worker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var javaScriptDeclaredCallablePattern = regexp.MustCompile(
	`(?:^|[;\n]\s*)(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)`,
)

func directCodingJavaScriptIdentifierChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failedStart int,
	failedEnd int,
	failed string,
	at *treesitter.Node,
	source []byte,
	bindings javaScriptLexicalBindings,
	external map[string]struct{},
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates := make([]directCodingIdentifierCandidate, 0, len(bindings.byName)+len(external))
	enclosingFunctionName := ""
	for parent := at.Parent(); parent != nil; parent = parent.Parent() {
		if !javaScriptFunctionScopeKind(parent.Kind()) {
			continue
		}
		if name := parent.ChildByFieldName("name"); name != nil {
			enclosingFunctionName = string(source[name.StartByte():name.EndByte()])
		}
		break
	}
	for name := range bindings.byName {
		if name == enclosingFunctionName {
			continue
		}
		if at == nil || bindings.referenceAllowed(name, at) {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: name, role: "lexically in-scope value",
			})
		}
	}
	for name := range external {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted direct value",
		})
	}
	candidates = directCodingTrialIdentifierCandidates(
		body, failedStart, failedEnd, candidates,
		func(trial string) error {
			_, err := validateDirectCodingJavaScriptFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices("JavaScript", failed, candidates)
}

func directCodingJavaScriptTokenChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	candidates []directCodingIdentifierCandidate,
) ([]assemblyline.OpaqueModelChoice, error) {
	if node == nil {
		return nil, fmt.Errorf("JavaScript closed choice requires one exact source token")
	}
	bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
	startByte := int(node.StartByte()) - bodyStart
	endByte := int(node.EndByte()) - bodyStart
	if startByte < 0 || endByte <= startByte || endByte > len(body) {
		return nil, fmt.Errorf("JavaScript closed choice token is outside the implementation body")
	}
	candidates = directCodingTrialIdentifierCandidates(
		body, startByte, endByte, candidates,
		func(trial string) error {
			_, err := validateDirectCodingJavaScriptFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices(
		"JavaScript", string(source[node.StartByte():node.EndByte()]), candidates,
	)
}

func directCodingJavaScriptCallableChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	root *treesitter.Node,
	source []byte,
	bindings javaScriptLexicalBindings,
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates := make([]directCodingIdentifierCandidate, 0)
	var inspect func(*treesitter.Node)
	inspect = func(current *treesitter.Node) {
		if current == nil {
			return
		}
		switch current.Kind() {
		case "function_declaration":
			name := current.ChildByFieldName("name")
			parent := current.Parent()
			if name != nil && parent != nil && parent.Kind() != "program" {
				value := string(source[name.StartByte():name.EndByte()])
				if bindings.referenceAllowed(value, node) {
					candidates = append(candidates, directCodingIdentifierCandidate{
						name: value, role: "in-scope callable",
					})
				}
			}
		case "variable_declarator":
			name := current.ChildByFieldName("name")
			value := current.ChildByFieldName("value")
			if name != nil && value != nil &&
				(value.Kind() == "arrow_function" || value.Kind() == "function_expression") {
				identifier := string(source[name.StartByte():name.EndByte()])
				if bindings.referenceAllowed(identifier, node) {
					candidates = append(candidates, directCodingIdentifierCandidate{
						name: identifier, role: "in-scope callable",
					})
				}
			}
		}
		for index := uint(0); index < current.NamedChildCount(); index++ {
			inspect(current.NamedChild(index))
		}
	}
	inspect(root)
	for _, authority := range append(
		append([]string(nil), input.Capabilities...), input.PermittedSymbols...,
	) {
		for _, match := range javaScriptDeclaredCallablePattern.FindAllStringSubmatch(authority, -1) {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: match[1], role: "permitted callable",
			})
		}
	}
	return directCodingJavaScriptTokenChoices(input, body, node, source, candidates)
}

type javaScriptPropertyChoiceForm uint8

const (
	javaScriptMemberPropertyChoice javaScriptPropertyChoiceForm = iota + 1
	javaScriptPatternPropertyChoice
	javaScriptSubscriptPropertyChoice
)

func directCodingJavaScriptPropertyChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	node *treesitter.Node,
	source []byte,
	computedKeys map[string]string,
	form javaScriptPropertyChoiceForm,
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates := make([]directCodingIdentifierCandidate, 0, len(computedKeys))
	for _, property := range computedKeys {
		property = strings.TrimSpace(property)
		if property == "" || javaScriptSensitiveProperty(property) {
			continue
		}
		replacement := ""
		switch form {
		case javaScriptMemberPropertyChoice:
			if javaScriptIdentifier(property) {
				replacement = property
			}
		case javaScriptPatternPropertyChoice:
			if javaScriptIdentifier(property) {
				replacement = property
			} else {
				replacement = strconv.Quote(property)
			}
		case javaScriptSubscriptPropertyChoice:
			replacement = strconv.Quote(property)
		}
		if replacement != "" {
			candidates = append(candidates, directCodingIdentifierCandidate{
				name: replacement, role: "permitted property",
			})
		}
	}
	return directCodingJavaScriptTokenChoices(input, body, node, source, candidates)
}
