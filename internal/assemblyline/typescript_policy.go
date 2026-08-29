package assemblyline

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateTypeScriptFunctionPolicyNode(
	declaration *treesitter.Node,
	source []byte,
	policy SourceFunctionPolicy,
) error {
	for _, identifier := range policy.ForbiddenIdentifiers {
		if containsTypeScriptIdentifier(declaration, source, identifier) {
			return fmt.Errorf("TypeScript fragment uses forbidden direct identifier %s", identifier)
		}
	}
	for _, element := range policy.RequiredElementNames {
		if !containsTypeScriptJSXElement(declaration, source, element) {
			return fmt.Errorf("TypeScript fragment requires a %s JSX element", element)
		}
	}
	for _, requirement := range policy.RequiredCalls {
		if !containsTypeScriptCall(declaration, source, requirement) {
			detail := strings.Join(requirement.Callees, " or ")
			if requirement.StringArgument != "" {
				detail += fmt.Sprintf(" with exact argument %q", requirement.StringArgument)
			}
			return fmt.Errorf("TypeScript fragment requires a call to %s", detail)
		}
	}
	for _, restriction := range policy.RestrictedCalls {
		if err := validateTypeScriptCallRestriction(declaration, source, restriction); err != nil {
			return err
		}
	}
	if err := validateTypeScriptTopLevelCalls(declaration, source, policy.TopLevelCalls); err != nil {
		return err
	}
	return nil
}

func validateTypeScriptTopLevelCalls(
	declaration *treesitter.Node,
	source []byte,
	callees []string,
) error {
	if declaration == nil || len(callees) == 0 {
		return nil
	}
	body := declaration.ChildByFieldName("body")
	if body == nil {
		return fmt.Errorf("TypeScript function has no body for top-level call validation")
	}
	var visit func(*treesitter.Node) error
	visit = func(node *treesitter.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind() == "call_expression" {
			callee := node.ChildByFieldName("function")
			if callee != nil && stringInSet(callee.Utf8Text(source), callees) &&
				!isDirectTypeScriptFunctionBodyCall(node, body) {
				return fmt.Errorf("TypeScript hook %s must be called at function top level", callee.Utf8Text(source))
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if err := visit(node.NamedChild(index)); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(declaration)
}

func isDirectTypeScriptFunctionBodyCall(call, body *treesitter.Node) bool {
	for ancestor := call.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		if ancestor.Id() == body.Id() {
			return true
		}
		switch ancestor.Kind() {
		case "statement_block", "arrow_function", "function_expression", "function_declaration",
			"generator_function", "method_definition", "if_statement", "switch_statement",
			"switch_case", "for_statement", "for_in_statement", "while_statement",
			"do_statement", "try_statement", "catch_clause", "finally_clause", "ternary_expression":
			return false
		}
	}
	return false
}

func validateTypeScriptCallRestriction(
	node *treesitter.Node,
	source []byte,
	restriction SourceCallRestriction,
) error {
	if node == nil {
		return nil
	}
	if node.Kind() == "call_expression" {
		callee := node.ChildByFieldName("function")
		if callee != nil && stringInSet(callee.Utf8Text(source), restriction.Callees) {
			arguments := node.ChildByFieldName("arguments")
			if arguments == nil || uint(restriction.StringArgumentIndex) >= arguments.NamedChildCount() {
				return fmt.Errorf("TypeScript fragment call to %s requires a code-owned string argument", callee.Utf8Text(source))
			}
			argument := arguments.NamedChild(uint(restriction.StringArgumentIndex))
			allowed := false
			for _, expected := range restriction.AllowedStringArguments {
				if argument != nil && exactTypeScriptString(argument.Utf8Text(source), expected) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("TypeScript fragment call to %s writes outside its code-owned capability channel", callee.Utf8Text(source))
			}
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if err := validateTypeScriptCallRestriction(node.NamedChild(index), source, restriction); err != nil {
			return err
		}
	}
	return nil
}

func containsTypeScriptIdentifier(node *treesitter.Node, source []byte, expected string) bool {
	if node == nil {
		return false
	}
	if strings.Contains(node.Kind(), "identifier") && node.Utf8Text(source) == expected {
		return true
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsTypeScriptIdentifier(node.NamedChild(index), source, expected) {
			return true
		}
	}
	return false
}

func containsTypeScriptJSXElement(node *treesitter.Node, source []byte, expected string) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "jsx_opening_element" || node.Kind() == "jsx_self_closing_element" {
		if name := node.ChildByFieldName("name"); name != nil && name.Utf8Text(source) == expected {
			return true
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsTypeScriptJSXElement(node.NamedChild(index), source, expected) {
			return true
		}
	}
	return false
}

func containsTypeScriptCall(
	node *treesitter.Node,
	source []byte,
	requirement SourceCallRequirement,
) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "call_expression" {
		callee := node.ChildByFieldName("function")
		if callee != nil && stringInSet(callee.Utf8Text(source), requirement.Callees) {
			if requirement.StringArgument == "" {
				return true
			}
			arguments := node.ChildByFieldName("arguments")
			if arguments != nil && uint(requirement.StringArgumentIndex) < arguments.NamedChildCount() {
				argument := arguments.NamedChild(uint(requirement.StringArgumentIndex))
				if argument != nil && exactTypeScriptString(argument.Utf8Text(source), requirement.StringArgument) {
					return true
				}
			}
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsTypeScriptCall(node.NamedChild(index), source, requirement) {
			return true
		}
	}
	return false
}

func exactTypeScriptString(raw, expected string) bool {
	if len(raw) < 2 {
		return false
	}
	quote := raw[0]
	return (quote == '\'' || quote == '"') && raw[len(raw)-1] == quote && raw[1:len(raw)-1] == expected
}

func stringInSet(value string, values []string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
