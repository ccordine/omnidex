package worker

import (
	"fmt"
	"strconv"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type javaScriptLexicalScope struct {
	start uint
	end   uint
}

type javaScriptLexicalBindings struct {
	declarations map[uintptr]struct{}
	byName       map[string][]javaScriptLexicalScope
}

func (bindings javaScriptLexicalBindings) declaration(node *treesitter.Node) bool {
	_, exists := bindings.declarations[node.Id()]
	return exists
}

func (bindings javaScriptLexicalBindings) referenceAllowed(
	name string,
	node *treesitter.Node,
) bool {
	for _, scope := range bindings.byName[name] {
		if node.StartByte() >= scope.start && node.EndByte() <= scope.end {
			return true
		}
	}
	return false
}

func collectJavaScriptLexicalBindings(
	root *treesitter.Node,
	source []byte,
	external map[string]struct{},
	forbidden map[string]struct{},
) (javaScriptLexicalBindings, error) {
	bindings := javaScriptLexicalBindings{
		declarations: make(map[uintptr]struct{}),
		byName:       make(map[string][]javaScriptLexicalScope),
	}
	rootScope := javaScriptLexicalScope{start: root.StartByte(), end: root.EndByte()}
	var collect func(*treesitter.Node, javaScriptLexicalScope, javaScriptLexicalScope) error
	collect = func(
		node *treesitter.Node,
		lexicalScope javaScriptLexicalScope,
		functionScope javaScriptLexicalScope,
	) error {
		if node == nil {
			return nil
		}
		kind := node.Kind()
		if javaScriptFunctionScopeKind(kind) {
			if kind == "function_declaration" {
				if err := bindings.addPattern(
					node.ChildByFieldName("name"), source, lexicalScope, external, forbidden,
				); err != nil {
					return err
				}
			}
			ownScope := javaScriptLexicalScope{start: node.StartByte(), end: node.EndByte()}
			if kind == "function_expression" {
				if err := bindings.addPattern(
					node.ChildByFieldName("name"), source, ownScope, external, forbidden,
				); err != nil {
					return err
				}
			}
			for _, field := range []string{"parameters", "parameter"} {
				if err := bindings.addPattern(
					node.ChildByFieldName(field), source, ownScope, external, forbidden,
				); err != nil {
					return err
				}
			}
			return collectJavaScriptChildren(node, func(child *treesitter.Node) error {
				return collect(child, ownScope, ownScope)
			})
		}
		if javaScriptBlockScopeKind(kind) {
			lexicalScope = javaScriptLexicalScope{start: node.StartByte(), end: node.EndByte()}
			if kind == "catch_clause" {
				if err := bindings.addPattern(
					node.ChildByFieldName("parameter"), source, lexicalScope, external, forbidden,
				); err != nil {
					return err
				}
			}
		}
		switch kind {
		case "lexical_declaration":
			if err := bindings.addVariableDeclarators(node, source, lexicalScope, external, forbidden); err != nil {
				return err
			}
		case "variable_declaration":
			if err := bindings.addVariableDeclarators(node, source, functionScope, external, forbidden); err != nil {
				return err
			}
		case "class_declaration":
			if err := bindings.addPattern(
				node.ChildByFieldName("name"), source, lexicalScope, external, forbidden,
			); err != nil {
				return err
			}
		}
		return collectJavaScriptChildren(node, func(child *treesitter.Node) error {
			return collect(child, lexicalScope, functionScope)
		})
	}
	if err := collect(root, rootScope, rootScope); err != nil {
		return javaScriptLexicalBindings{}, err
	}
	return bindings, nil
}

func (bindings javaScriptLexicalBindings) addVariableDeclarators(
	node *treesitter.Node,
	source []byte,
	scope javaScriptLexicalScope,
	external map[string]struct{},
	forbidden map[string]struct{},
) error {
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}
		if err := bindings.addPattern(
			child.ChildByFieldName("name"), source, scope, external, forbidden,
		); err != nil {
			return err
		}
	}
	return nil
}

func (bindings javaScriptLexicalBindings) addPattern(
	node *treesitter.Node,
	source []byte,
	scope javaScriptLexicalScope,
	external map[string]struct{},
	forbidden map[string]struct{},
) error {
	if node == nil {
		return nil
	}
	if node.Kind() == "identifier" || node.Kind() == "shorthand_property_identifier_pattern" {
		name := string(source[node.StartByte():node.EndByte()])
		if _, denied := forbidden[name]; denied {
			return fmt.Errorf("JavaScript fragment binds forbidden identifier %s", name)
		}
		if _, shadows := external[name]; shadows {
			return fmt.Errorf("JavaScript fragment shadows permitted direct symbol %s", name)
		}
		bindings.declarations[node.Id()] = struct{}{}
		bindings.byName[name] = append(bindings.byName[name], scope)
		return nil
	}
	if node.Kind() == "assignment_pattern" {
		left := node.ChildByFieldName("left")
		if left == nil && node.NamedChildCount() > 0 {
			left = node.NamedChild(0)
		}
		return bindings.addPattern(left, source, scope, external, forbidden)
	}
	if node.Kind() == "pair_pattern" {
		return bindings.addPattern(
			node.ChildByFieldName("value"), source, scope, external, forbidden,
		)
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if err := bindings.addPattern(
			node.NamedChild(index), source, scope, external, forbidden,
		); err != nil {
			return err
		}
	}
	return nil
}

func collectJavaScriptChildren(
	node *treesitter.Node,
	visit func(*treesitter.Node) error,
) error {
	for index := uint(0); index < node.ChildCount(); index++ {
		if err := visit(node.Child(index)); err != nil {
			return err
		}
	}
	return nil
}

func javaScriptFunctionScopeKind(kind string) bool {
	switch kind {
	case "function_declaration", "function_expression", "arrow_function", "method_definition",
		"generator_function_declaration", "generator_function":
		return true
	default:
		return false
	}
}

func javaScriptBlockScopeKind(kind string) bool {
	switch kind {
	case "statement_block", "catch_clause", "for_statement", "for_in_statement", "switch_statement":
		return true
	default:
		return false
	}
}

func javaScriptStaticPropertyName(source []byte, node *treesitter.Node) (string, bool) {
	if node == nil {
		return "", false
	}
	text := strings.TrimSpace(string(source[node.StartByte():node.EndByte()]))
	switch node.Kind() {
	case "string":
		if len(text) < 2 {
			return "", false
		}
		if text[0] == '\'' && text[len(text)-1] == '\'' {
			text = `"` + strings.ReplaceAll(text[1:len(text)-1], `"`, `\"`) + `"`
		}
		value, err := strconv.Unquote(text)
		return value, err == nil
	case "template_string":
		if strings.Contains(text, "${") || len(text) < 2 {
			return "", false
		}
		quoted := `"` + strings.ReplaceAll(text[1:len(text)-1], `"`, `\"`) + `"`
		value, err := strconv.Unquote(quoted)
		return value, err == nil
	case "parenthesized_expression":
		if node.NamedChildCount() == 1 {
			return javaScriptStaticPropertyName(source, node.NamedChild(0))
		}
	case "binary_expression":
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		leftValue, leftOK := javaScriptStaticPropertyName(source, left)
		rightValue, rightOK := javaScriptStaticPropertyName(source, right)
		if leftOK && rightOK && strings.Contains(text, "+") {
			return leftValue + rightValue, true
		}
	}
	return "", false
}
