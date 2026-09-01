package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescriptgrammar "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func directCodingTypeScriptIdentifierChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failed string,
	failedStart int,
	failedEnd int,
	tsx bool,
	policy assemblyline.SourceFunctionPolicy,
) ([]assemblyline.OpaqueModelChoice, error) {
	declaration, err := assemblyline.ComposeSourceDeclaration(input.Signature, body)
	if err != nil {
		return nil, err
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	language := typescriptgrammar.LanguageTypescript()
	if tsx {
		language = typescriptgrammar.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		return nil, err
	}
	tree := parser.Parse([]byte(declaration), nil)
	if tree == nil {
		return nil, fmt.Errorf("TypeScript scope parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, fmt.Errorf("TypeScript scope parser rejected the declaration")
	}
	candidates := make([]directCodingIdentifierCandidate, 0)
	bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
	absoluteStart, absoluteEnd := bodyStart+failedStart, bodyStart+failedEnd
	var failedNode *treesitter.Node
	var findFailed func(*treesitter.Node)
	findFailed = func(node *treesitter.Node) {
		if node == nil || failedNode != nil {
			return
		}
		if int(node.StartByte()) == absoluteStart && int(node.EndByte()) == absoluteEnd &&
			node.Kind() == "identifier" {
			failedNode = node
			return
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			findFailed(node.NamedChild(index))
		}
	}
	findFailed(root)
	if failedNode == nil {
		return nil, fmt.Errorf("TypeScript scope parser could not bind the failed identifier")
	}
	var collectPattern func(*treesitter.Node, string)
	collectPattern = func(node *treesitter.Node, role string) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "identifier", "shorthand_property_identifier_pattern":
			if directCodingTreeBindingAvailableAt(
				node, failedNode, directCodingTypeScriptScopeKind,
			) {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: string([]byte(declaration)[node.StartByte():node.EndByte()]),
					role: role,
				})
			}
			return
		case "type_annotation", "predefined_type", "type_identifier", "generic_type":
			return
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			collectPattern(node.NamedChild(index), role)
		}
	}
	var inspect func(*treesitter.Node)
	inspect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "function_declaration", "function_expression", "arrow_function":
			collectPattern(node.ChildByFieldName("parameters"), "function parameter")
		case "variable_declarator":
			collectPattern(node.ChildByFieldName("name"), "local value")
		case "catch_clause":
			collectPattern(node.ChildByFieldName("parameter"), "local caught value")
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			inspect(node.NamedChild(index))
		}
	}
	inspect(root)
	for name := range directCodingExplicitIdentifierAuthorities(input, javaScriptIdentifier) {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted direct value",
		})
	}
	candidates = directCodingTrialIdentifierCandidates(
		body, failedStart, failedEnd, candidates,
		func(trial string) error {
			_, err := assemblyline.ParseTypeScriptFunctionBody(
				assemblyline.TypeScriptFunctionContract{
					Signature: input.Signature, TSX: tsx, Policy: policy,
				},
				trial,
			)
			return err
		},
	)
	return directCodingIdentifierChoices("TypeScript", failed, candidates)
}

func directCodingTypeScriptScopeKind(kind string) bool {
	switch kind {
	case "statement_block", "function_declaration", "function_expression", "arrow_function",
		"generator_function_declaration", "generator_function", "catch_clause",
		"for_statement", "for_in_statement":
		return true
	default:
		return false
	}
}
