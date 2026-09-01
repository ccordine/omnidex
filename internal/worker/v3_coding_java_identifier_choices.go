package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func directCodingJavaIdentifierChoices(
	input assemblyline.FragmentGenerationInput,
	body string,
	failedStart int,
	failedEnd int,
	failed string,
	root *treesitter.Node,
	source []byte,
	at *treesitter.Node,
	external map[string]struct{},
) ([]assemblyline.OpaqueModelChoice, error) {
	candidates := make([]directCodingIdentifierCandidate, 0, len(external))
	var addBinding func(*treesitter.Node, string)
	addBinding = func(node *treesitter.Node, role string) {
		if node == nil {
			return
		}
		if node.Kind() == "identifier" {
			if directCodingTreeBindingAvailableAt(node, at, directCodingJavaScopeKind) {
				candidates = append(candidates, directCodingIdentifierCandidate{
					name: javaNodeText(node, source), role: role,
				})
			}
			return
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			addBinding(node.NamedChild(index), role)
		}
	}
	var inspect func(*treesitter.Node)
	inspect = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "formal_parameter", "spread_parameter", "catch_formal_parameter", "type_pattern":
			addBinding(node.ChildByFieldName("name"), "function parameter")
		case "variable_declarator":
			addBinding(node.ChildByFieldName("name"), "local value")
		case "enhanced_for_statement":
			addBinding(node.ChildByFieldName("name"), "local iteration value")
		case "lambda_expression":
			addBinding(node.ChildByFieldName("parameters"), "function parameter")
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			inspect(node.NamedChild(index))
		}
	}
	inspect(root)
	for name := range external {
		candidates = append(candidates, directCodingIdentifierCandidate{
			name: name, role: "permitted direct value",
		})
	}
	candidates = directCodingTrialIdentifierCandidates(
		body, failedStart, failedEnd, candidates,
		func(trial string) error {
			_, err := validateDirectCodingJavaFragment(input, trial)
			return err
		},
	)
	return directCodingIdentifierChoices("Java", failed, candidates)
}

func directCodingJavaPermittedValueAuthorities(
	input assemblyline.FragmentGenerationInput,
) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for index, authority := range append(
		append([]string(nil), input.Capabilities...), input.PermittedSymbols...,
	) {
		text := strings.TrimSpace(authority)
		if javaSourceIdentifier(text) {
			result[text] = struct{}{}
			continue
		}
		source := []byte("final class OmnidexCapabilityScope {\n" + text + "\n}")
		parser := treesitter.NewParser()
		if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
			parser.Close()
			return nil, err
		}
		tree := parser.Parse(source, nil)
		if tree == nil || tree.RootNode() == nil || tree.RootNode().HasError() {
			if tree != nil {
				tree.Close()
			}
			parser.Close()
			return nil, fmt.Errorf("parse permitted Java value %d", index)
		}
		var inspect func(*treesitter.Node)
		inspect = func(node *treesitter.Node) {
			if node == nil {
				return
			}
			if node.Kind() == "variable_declarator" {
				parent := node.Parent()
				if parent != nil && parent.Kind() == "field_declaration" {
					if name := node.ChildByFieldName("name"); name != nil {
						result[javaNodeText(name, source)] = struct{}{}
					}
				}
			}
			for child := uint(0); child < node.NamedChildCount(); child++ {
				inspect(node.NamedChild(child))
			}
		}
		inspect(tree.RootNode())
		tree.Close()
		parser.Close()
	}
	return result, nil
}

func directCodingJavaScopeKind(kind string) bool {
	switch kind {
	case "block", "method_declaration", "constructor_declaration", "lambda_expression",
		"catch_clause", "enhanced_for_statement", "for_statement":
		return true
	default:
		return false
	}
}
