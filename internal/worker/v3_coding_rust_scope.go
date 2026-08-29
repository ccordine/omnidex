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
	if err := validateRustFragmentAuthority(input, []byte(source)); err != nil {
		return "", err
	}
	return source, nil
}

func validateRustFragmentAuthority(
	input assemblyline.FragmentGenerationInput,
	source []byte,
) error {
	root, closeTree, err := parseRustAuthorityTree(source)
	if err != nil {
		return err
	}
	defer closeTree()
	permitted, err := rustPermittedDeclarationNames(input.PermittedSymbols)
	if err != nil {
		return err
	}
	locals, bindings, err := rustFragmentLocalBindings(source, root, permitted)
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
		case "use_declaration", "extern_crate_declaration", "macro_definition",
			"unsafe_block", "attribute_item", "inner_attribute_item", "const_item",
			"static_item", "struct_item", "enum_item", "type_item", "trait_item",
			"impl_item", "mod_item", "foreign_mod_item":
			authorityErr = fmt.Errorf("Rust fragment contains forbidden %s authority", node.Kind())
		case "macro_invocation":
			authorityErr = validateRustFragmentMacro(source, node, permitted)
		case "call_expression":
			authorityErr = validateRustFragmentCall(source, node, locals, permitted)
		case "scoped_identifier", "scoped_type_identifier":
			authorityErr = validateRustFragmentPath(rustNodeText(source, node), locals, permitted)
		case "type_identifier":
			name := rustNodeText(source, node)
			if !rustFragmentSymbolAllowed(name, locals, permitted) {
				authorityErr = fmt.Errorf("Rust fragment type %s is outside declared authority", name)
			}
		case "identifier":
			name := rustNodeText(source, node)
			if rustFragmentForbiddenSymbol(name) {
				authorityErr = fmt.Errorf("Rust fragment uses forbidden environment authority %s", name)
				return
			}
			if _, binding := bindings[node.Id()]; binding || rustIdentifierBelongsToPath(node, parent) ||
				rustIdentifierIsMemberToken(source, node) {
				return
			}
			if !rustFragmentSymbolAllowed(name, locals, permitted) {
				authorityErr = fmt.Errorf("Rust fragment symbol %s is outside declared authority", name)
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

func rustPermittedDeclarationNames(values []string) (map[string]struct{}, error) {
	allowed := make(map[string]struct{})
	for index, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			return nil, fmt.Errorf("Rust permitted API %d is empty", index)
		}
		if rustSimpleIdentifier(text) {
			allowed[text] = struct{}{}
			continue
		}
		parsedText := text
		root, closeTree, err := parseRustAuthorityTree([]byte(parsedText))
		if err != nil && !strings.Contains(text, "\n") && strings.Contains(text, "fn ") {
			parsedText = text + " {}"
			root, closeTree, err = parseRustAuthorityTree([]byte(parsedText))
		}
		if err != nil {
			return nil, fmt.Errorf("parse Rust permitted API %d: %w", index, err)
		}
		declarations := 0
		walkRustTree(root, func(node *treesitter.Node) {
			switch node.Kind() {
			case "function_item", "struct_item", "enum_item", "type_item", "const_item", "static_item", "trait_item":
				name := node.ChildByFieldName("name")
				if name != nil {
					allowed[rustNodeText([]byte(parsedText), name)] = struct{}{}
					declarations++
				}
			}
		})
		closeTree()
		if declarations == 0 {
			return nil, fmt.Errorf("Rust permitted API %d declares no callable, type, or constant", index)
		}
	}
	return allowed, nil
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
