package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingRustAuthorityCatalog struct {
	allowed   map[string]struct{}
	values    map[string]struct{}
	functions map[string]int
	macros    map[string]struct{}
	pathRoots map[string]struct{}
	types     map[string]struct{}
}

func newDirectCodingRustAuthorityCatalog(
	input assemblyline.FragmentGenerationInput,
) (directCodingRustAuthorityCatalog, error) {
	catalog := directCodingRustAuthorityCatalog{
		allowed: make(map[string]struct{}), values: make(map[string]struct{}),
		functions: make(map[string]int), macros: make(map[string]struct{}),
		pathRoots: make(map[string]struct{}), types: make(map[string]struct{}),
	}
	for index, declaration := range input.Capabilities {
		if err := catalog.addDeclaration(declaration, "capability", index); err != nil {
			return directCodingRustAuthorityCatalog{}, err
		}
	}
	for index, symbol := range input.PermittedSymbols {
		text := strings.TrimSpace(symbol)
		if rustSimpleIdentifier(text) {
			// A raw registered symbol proves only direct value availability. It
			// does not prove that the same spelling is a macro, function, type,
			// or path root in Rust's separate namespaces.
			catalog.allowed[text] = struct{}{}
			catalog.values[text] = struct{}{}
			continue
		}
		if err := catalog.addDeclaration(symbol, "permitted symbol", index); err != nil {
			return directCodingRustAuthorityCatalog{}, err
		}
	}
	return catalog, nil
}

func (catalog directCodingRustAuthorityCatalog) addDeclaration(
	value string,
	label string,
	index int,
) error {
	text := strings.TrimSpace(value)
	if text == "" {
		return fmt.Errorf("Rust %s %d is empty", label, index)
	}
	if rustSimpleIdentifier(text) {
		catalog.allowed[text] = struct{}{}
		catalog.values[text] = struct{}{}
		return nil
	}
	parsedText := text
	root, closeTree, err := parseRustAuthorityTree([]byte(parsedText))
	if err != nil && !strings.Contains(text, "\n") && strings.Contains(text, "fn ") {
		parsedText = text + " {}"
		root, closeTree, err = parseRustAuthorityTree([]byte(parsedText))
	}
	if err != nil {
		return fmt.Errorf("parse Rust %s %d: %w", label, index, err)
	}
	declarations := 0
	walkRustTree(root, func(node *treesitter.Node) {
		if !rustTopLevelAuthorityDeclaration(node) {
			return
		}
		name := node.ChildByFieldName("name")
		if name == nil {
			return
		}
		identifier := rustNodeText([]byte(parsedText), name)
		switch node.Kind() {
		case "function_item":
			catalog.allowed[identifier] = struct{}{}
			catalog.values[identifier] = struct{}{}
			catalog.functions[identifier] = rustParameterCount(node)
			declarations++
		case "macro_definition":
			catalog.allowed[identifier] = struct{}{}
			catalog.macros[identifier] = struct{}{}
			declarations++
		case "struct_item", "enum_item", "union_item", "type_item", "trait_item":
			catalog.allowed[identifier] = struct{}{}
			catalog.pathRoots[identifier] = struct{}{}
			catalog.types[identifier] = struct{}{}
			declarations++
		case "mod_item":
			catalog.allowed[identifier] = struct{}{}
			catalog.pathRoots[identifier] = struct{}{}
			declarations++
		case "const_item", "static_item":
			catalog.allowed[identifier] = struct{}{}
			catalog.values[identifier] = struct{}{}
			declarations++
		}
	})
	closeTree()
	if declarations == 0 {
		return fmt.Errorf(
			"Rust %s %d declares no callable, macro, path root, type, or value",
			label, index,
		)
	}
	return nil
}

func rustTopLevelAuthorityDeclaration(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	parent := node.Parent()
	return parent != nil && parent.Kind() == "source_file"
}

func rustParameterCount(node *treesitter.Node) int {
	if node == nil {
		return -1
	}
	if node.Kind() == "parameters" || node.Kind() == "closure_parameters" {
		return int(node.NamedChildCount())
	}
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return -1
	}
	return int(parameters.NamedChildCount())
}
