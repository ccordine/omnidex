package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	rustgrammar "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

func validateDirectCodingRustFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	source, err := assemblyline.ValidateRustFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	if err := validateRustFragmentAuthority(input, candidate, []byte(source)); err != nil {
		return "", directCodingSourceBodyError(input, candidate, source, err)
	}
	return source, nil
}

func validateRustFragmentAuthority(
	input assemblyline.FragmentGenerationInput,
	body string,
	source []byte,
) error {
	root, closeTree, err := parseRustAuthorityTree(source)
	if err != nil {
		return err
	}
	defer closeTree()
	catalog, err := newDirectCodingRustAuthorityCatalog(input)
	if err != nil {
		return err
	}
	locals, bindings, err := rustFragmentLocalBindings(source, root, catalog.allowed)
	if err != nil {
		return err
	}
	functionItems := 0
	var authorityErr error
	walkRustTreeWithParent(root, nil, func(node, parent *treesitter.Node) {
		if authorityErr != nil {
			return
		}
		switch node.Kind() {
		case "function_item":
			functionItems++
			if functionItems > 1 {
				authorityErr = directCodingSourceNodeError(
					node,
					"What in-scope implementation should replace this nested declaration?",
					fmt.Errorf("Rust fragment contains more than one function declaration"),
				)
			}
		case "use_declaration", "extern_crate_declaration", "macro_definition",
			"unsafe_block", "attribute_item", "inner_attribute_item", "const_item",
			"static_item", "struct_item", "enum_item", "type_item", "trait_item",
			"impl_item", "mod_item", "foreign_mod_item":
			authorityErr = directCodingSourceNodeError(
				node,
				"What in-scope implementation should replace this unavailable declaration?",
				fmt.Errorf("Rust fragment contains forbidden %s authority", node.Kind()),
			)
		case "macro_invocation":
			if failure := validateRustFragmentMacro(source, node, root, catalog); failure != nil {
				authorityErr = directCodingRustAuthorityNodeError(input, body, failure)
			}
		case "call_expression":
			if failure := validateRustFragmentCall(source, node, root, catalog); failure != nil {
				authorityErr = directCodingRustAuthorityNodeError(input, body, failure)
			}
		case "scoped_identifier", "scoped_type_identifier":
			if failure := validateRustFragmentPath(source, node, root, catalog); failure != nil {
				authorityErr = directCodingRustAuthorityNodeError(input, body, failure)
			}
		case "type_identifier":
			if rustTypeIdentifierBelongsToPath(node, parent) {
				return
			}
			name := rustNodeText(source, node)
			candidates := directCodingRustTypeCandidates(root, source, catalog)
			if !directCodingRustCandidateNamed(candidates, name) {
				authorityErr = directCodingRustAuthorityNodeError(
					input,
					body,
					&directCodingRustAuthorityFailure{
						node:       node,
						question:   "Which available type satisfies this type position?",
						candidates: candidates,
						cause:      fmt.Errorf("Rust fragment type %s is outside declared type authority", name),
					},
				)
			}
		case "identifier":
			name := rustNodeText(source, node)
			bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
			failedStart := int(node.StartByte()) - bodyStart
			failedEnd := int(node.EndByte()) - bodyStart
			if rustFragmentForbiddenSymbol(name) {
				replacements, replacementErr := directCodingRustIdentifierChoices(
					input, body, failedStart, failedEnd,
					name, root, source, node, bindings, catalog,
				)
				if replacementErr != nil {
					authorityErr = replacementErr
					return
				}
				authorityErr = directCodingIdentifierNodeError(
					node,
					"Which available value has the meaning required at this unavailable reference?",
					replacements,
					fmt.Errorf("Rust fragment uses forbidden environment authority %s", name),
				)
				return
			}
			if _, binding := bindings[node.Id()]; binding || rustIdentifierBelongsToPath(node, parent) ||
				rustIdentifierIsMemberToken(source, node) {
				return
			}
			if !rustFragmentSymbolAllowed(name, locals, catalog.allowed) {
				replacements, replacementErr := directCodingRustIdentifierChoices(
					input, body, failedStart, failedEnd,
					name, root, source, node, bindings, catalog,
				)
				if replacementErr != nil {
					authorityErr = replacementErr
					return
				}
				authorityErr = directCodingIdentifierNodeError(
					node,
					"Which available value has the meaning required at this unresolved reference?",
					replacements,
					fmt.Errorf("Rust fragment symbol %s is outside declared authority", name),
				)
			}
		}
	})
	if authorityErr != nil {
		return authorityErr
	}
	if functionItems != 1 {
		return fmt.Errorf("Rust fragment contains %d function declarations", functionItems)
	}
	return nil
}

func directCodingRustAuthorityNodeError(
	input assemblyline.FragmentGenerationInput,
	body string,
	failure *directCodingRustAuthorityFailure,
) error {
	if failure == nil || failure.cause == nil {
		return nil
	}
	if failure.node == nil || failure.question == "" {
		return failure.cause
	}
	bodyStart := len(strings.TrimSpace(input.Signature) + " {\n")
	failedStart := int(failure.node.StartByte()) - bodyStart
	failedEnd := int(failure.node.EndByte()) - bodyStart
	if failedStart < 0 || failedEnd <= failedStart || failedEnd > len(body) {
		return failure.cause
	}
	replacements, err := directCodingRustValidatedChoices(
		input,
		body,
		failedStart,
		failedEnd,
		body[failedStart:failedEnd],
		failure.candidates,
	)
	if err != nil {
		return fmt.Errorf("enumerate exact Rust replacement: %w", err)
	}
	return directCodingIdentifierNodeError(
		failure.node, failure.question, replacements, failure.cause,
	)
}

func parseRustAuthorityTree(
	source []byte,
) (*treesitter.Node, func(), error) {
	parser := treesitter.NewParser()
	if err := parser.SetLanguage(treesitter.NewLanguage(rustgrammar.Language())); err != nil {
		parser.Close()
		return nil, nil, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		parser.Close()
		return nil, nil, fmt.Errorf("Rust authority parser returned no tree")
	}
	root := tree.RootNode()
	if root == nil || root.HasError() {
		tree.Close()
		parser.Close()
		return nil, nil, fmt.Errorf("Rust authority source is not parseable")
	}
	return root, func() { tree.Close(); parser.Close() }, nil
}

func rustFragmentLocalBindings(
	source []byte,
	root *treesitter.Node,
	permitted map[string]struct{},
) (map[string]struct{}, map[uintptr]struct{}, error) {
	locals := make(map[string]struct{})
	bindings := make(map[uintptr]struct{})
	var bindingErr error
	addPattern := func(pattern *treesitter.Node) {
		collectRustPatternBindings(source, pattern, locals, bindings)
	}
	walkRustTree(root, func(node *treesitter.Node) {
		if bindingErr != nil {
			return
		}
		switch node.Kind() {
		case "function_item":
			name := node.ChildByFieldName("name")
			if name != nil {
				locals[rustNodeText(source, name)] = struct{}{}
				bindings[name.Id()] = struct{}{}
			}
		case "parameter", "let_declaration", "let_condition", "for_expression", "match_arm":
			addPattern(node.ChildByFieldName("pattern"))
		case "closure_parameters":
			for index := uint(0); index < node.NamedChildCount(); index++ {
				child := node.NamedChild(index)
				if child != nil && child.Kind() == "parameter" {
					addPattern(child.ChildByFieldName("pattern"))
				} else {
					addPattern(child)
				}
			}
		}
	})
	for name := range locals {
		if rustFragmentForbiddenSymbol(name) {
			bindingErr = fmt.Errorf("Rust fragment binds forbidden authority name %s", name)
			break
		}
		if _, shadows := permitted[name]; shadows {
			bindingErr = fmt.Errorf("Rust fragment shadows permitted capability %s", name)
			break
		}
		if rustFragmentPreludeSymbol(name) && name != "input" && name != "dependencies" {
			bindingErr = fmt.Errorf("Rust fragment shadows predeclared symbol %s", name)
			break
		}
	}
	return locals, bindings, bindingErr
}

func collectRustPatternBindings(
	source []byte,
	node *treesitter.Node,
	locals map[string]struct{},
	bindings map[uintptr]struct{},
) {
	if node == nil {
		return
	}
	if node.Kind() == "identifier" || node.Kind() == "shorthand_field_identifier" {
		locals[rustNodeText(source, node)] = struct{}{}
		bindings[node.Id()] = struct{}{}
		return
	}
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("name")
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child == nil || typeNode != nil && child.Id() == typeNode.Id() {
			continue
		}
		if node.Kind() == "field_pattern" && nameNode != nil && child.Id() == nameNode.Id() &&
			node.ChildByFieldName("pattern") != nil {
			continue
		}
		collectRustPatternBindings(source, child, locals, bindings)
	}
}

func walkRustTreeWithParent(
	node *treesitter.Node,
	parent *treesitter.Node,
	visit func(*treesitter.Node, *treesitter.Node),
) {
	if node == nil {
		return
	}
	visit(node, parent)
	for index := uint(0); index < node.ChildCount(); index++ {
		walkRustTreeWithParent(node.Child(index), node, visit)
	}
}
