package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserRuntimeBinding struct {
	name  string
	start uint
	end   uint
}

type directCodingBrowserRuntimeBindings struct {
	declarations map[uintptr]struct{}
	byName       map[string][]directCodingBrowserRuntimeBinding
}

func (bindings directCodingBrowserRuntimeBindings) binds(
	name string,
	reference *treesitter.Node,
) bool {
	if reference == nil {
		return false
	}
	if _, declaration := bindings.declarations[reference.Id()]; declaration {
		return true
	}
	for _, binding := range bindings.byName[name] {
		if reference.StartByte() >= binding.start && reference.EndByte() <= binding.end {
			return true
		}
	}
	return false
}

func collectDirectCodingBrowserRuntimeBindings(
	root *treesitter.Node,
	source []byte,
) (directCodingBrowserRuntimeBindings, error) {
	bindings := directCodingBrowserRuntimeBindings{
		declarations: make(map[uintptr]struct{}),
		byName:       make(map[string][]directCodingBrowserRuntimeBinding),
	}
	add := func(pattern, scope *treesitter.Node) error {
		return bindings.addPattern(pattern, scope, source)
	}
	var walk func(*treesitter.Node) error
	walk = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		switch node.Kind() {
		case "function_declaration":
			if err := add(node.ChildByFieldName("name"), directCodingBrowserRuntimeParentScope(node)); err != nil {
				return err
			}
			if err := add(node.ChildByFieldName("parameters"), node); err != nil {
				return err
			}
		case "function_expression":
			if err := add(node.ChildByFieldName("name"), node); err != nil {
				return err
			}
			if err := add(node.ChildByFieldName("parameters"), node); err != nil {
				return err
			}
		case "arrow_function":
			parameters := node.ChildByFieldName("parameters")
			if parameters == nil {
				parameters = node.ChildByFieldName("parameter")
			}
			if err := add(parameters, node); err != nil {
				return err
			}
		case "variable_declarator":
			declaration := directCodingBrowserRuntimeDeclaration(node)
			scope := directCodingBrowserRuntimeLexicalScope(declaration)
			if declaration != nil && declaration.Kind() == "variable_declaration" {
				scope = directCodingBrowserRuntimeFunctionScope(declaration)
			}
			if err := add(node.ChildByFieldName("name"), scope); err != nil {
				return err
			}
		case "class_declaration":
			if err := add(node.ChildByFieldName("name"), directCodingBrowserRuntimeParentScope(node)); err != nil {
				return err
			}
		case "catch_clause":
			if err := add(node.ChildByFieldName("parameter"), node); err != nil {
				return err
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := walk(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return directCodingBrowserRuntimeBindings{}, err
	}
	return bindings, nil
}

func (bindings *directCodingBrowserRuntimeBindings) addPattern(
	pattern *treesitter.Node,
	scope *treesitter.Node,
	source []byte,
) error {
	if pattern == nil {
		return nil
	}
	if scope == nil {
		return fmt.Errorf("browser runtime binding has no lexical scope")
	}
	switch pattern.Kind() {
	case "identifier", "shorthand_property_identifier_pattern":
		name := directCodingBrowserRuntimeNodeText(source, pattern)
		if name == "" {
			return fmt.Errorf("browser runtime binding has an empty identifier")
		}
		bindings.declarations[pattern.Id()] = struct{}{}
		bindings.byName[name] = append(bindings.byName[name], directCodingBrowserRuntimeBinding{
			name: name, start: scope.StartByte(), end: scope.EndByte(),
		})
		return nil
	case "required_parameter", "optional_parameter":
		value := pattern.ChildByFieldName("pattern")
		if value == nil && pattern.NamedChildCount() > 0 {
			value = pattern.NamedChild(0)
		}
		return bindings.addPattern(value, scope, source)
	case "assignment_pattern", "object_assignment_pattern":
		return bindings.addPattern(pattern.ChildByFieldName("left"), scope, source)
	case "rest_pattern":
		value := pattern.ChildByFieldName("argument")
		if value == nil && pattern.NamedChildCount() > 0 {
			value = pattern.NamedChild(0)
		}
		return bindings.addPattern(value, scope, source)
	case "pair_pattern":
		return bindings.addPattern(pattern.ChildByFieldName("value"), scope, source)
	case "formal_parameters", "array_pattern", "object_pattern":
		for index := uint(0); index < pattern.NamedChildCount(); index++ {
			if err := bindings.addPattern(pattern.NamedChild(index), scope, source); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func directCodingBrowserRuntimeDeclaration(node *treesitter.Node) *treesitter.Node {
	for current := node; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "lexical_declaration", "variable_declaration":
			return current
		case "statement_block", "program":
			return nil
		}
	}
	return nil
}

func directCodingBrowserRuntimeParentScope(node *treesitter.Node) *treesitter.Node {
	if node == nil {
		return nil
	}
	return directCodingBrowserRuntimeLexicalScope(node.Parent())
}

func directCodingBrowserRuntimeLexicalScope(node *treesitter.Node) *treesitter.Node {
	for current := node; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "statement_block", "program", "catch_clause", "for_statement",
			"for_in_statement", "for_of_statement", "switch_statement":
			return current
		case "function_declaration", "function_expression", "arrow_function":
			return current
		}
	}
	return nil
}

func directCodingBrowserRuntimeFunctionScope(node *treesitter.Node) *treesitter.Node {
	for current := node; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "function_declaration", "function_expression", "arrow_function", "program":
			return current
		}
	}
	return nil
}
