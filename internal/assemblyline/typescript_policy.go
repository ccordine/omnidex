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
		reference, uncorrectable := findTypeScriptForbiddenIdentifierUses(
			declaration, source, identifier,
		)
		if uncorrectable != nil {
			return newTypeScriptFragmentViolation(
				TypeScriptViolationForbiddenIdentifier,
				fmt.Sprintf(
					"TypeScript fragment uses forbidden identifier %s in a non-value-reference role",
					identifier,
				),
			)
		}
		if reference != nil {
			return newLocatedTypeScriptFragmentViolation(
				TypeScriptViolationForbiddenIdentifier,
				fmt.Sprintf("TypeScript fragment uses forbidden direct identifier %s", identifier),
				int(reference.StartByte()),
				int(reference.EndByte()),
			)
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

// findTypeScriptForbiddenIdentifierUses separates one direct runtime value
// reference from identifier spellings whose syntactic role cannot be changed
// by an exact one-token value splice. Properties and type names are different
// semantic categories and do not match this value-authority policy. Bindings,
// shorthand values, JSX names, and type queries still fail the policy, but
// remain deliberately unlocated so they cannot authorize an invalid value
// replacement.
func findTypeScriptForbiddenIdentifierUses(
	node *treesitter.Node,
	source []byte,
	expected string,

) (reference *treesitter.Node, uncorrectable *treesitter.Node) {
	if node == nil {
		return nil, nil
	}
	if node.Utf8Text(source) == expected {
		switch node.Kind() {
		case "identifier":
			if typeScriptIdentifierIsUncorrectableValueRole(node) {
				uncorrectable = node
			} else if !typeScriptIdentifierIsPropertyRole(node) {
				reference = node
			}
		case "shorthand_property_identifier", "shorthand_property_identifier_pattern":
			uncorrectable = node
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		childReference, childUncorrectable := findTypeScriptForbiddenIdentifierUses(
			node.NamedChild(index), source, expected,
		)
		if uncorrectable == nil {
			uncorrectable = childUncorrectable
		}
		if reference == nil {
			reference = childReference
		}
	}
	return reference, uncorrectable
}

func typeScriptIdentifierIsPropertyRole(node *treesitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	switch parent.Kind() {
	case "member_expression":
		return typeScriptNodeOccupiesField(node, parent, "property")
	case "pair", "pair_pattern", "method_definition", "public_field_definition":
		return typeScriptNodeOccupiesField(node, parent, "key", "name", "property")
	default:
		return false
	}
}

func typeScriptIdentifierIsUncorrectableValueRole(node *treesitter.Node) bool {
	parent := node.Parent()
	if parent == nil {
		return true
	}
	switch parent.Kind() {
	case "variable_declarator":
		return typeScriptNodeOccupiesField(node, parent, "name")
	case "function_declaration", "function_expression", "generator_function_declaration",
		"generator_function", "class_declaration", "class", "interface_declaration",
		"type_alias_declaration", "enum_declaration", "internal_module", "module":
		return typeScriptNodeOccupiesField(node, parent, "name", "parameter", "parameters")
	case "arrow_function":
		return typeScriptNodeOccupiesField(node, parent, "parameter", "parameters")
	case "required_parameter", "optional_parameter", "rest_pattern", "object_pattern",
		"array_pattern", "assignment_pattern", "catch_clause", "import_clause",
		"import_specifier", "namespace_import", "namespace_export", "import_alias",
		"export_specifier", "type_query", "type_predicate", "nested_identifier",
		"nested_type_identifier", "jsx_opening_element", "jsx_closing_element",
		"jsx_self_closing_element", "jsx_attribute":
		return true
	default:
		return false
	}
}

func typeScriptNodeOccupiesField(
	node *treesitter.Node,
	parent *treesitter.Node,
	fields ...string,
) bool {
	if node == nil || parent == nil {
		return false
	}
	for _, field := range fields {
		candidate := parent.ChildByFieldName(field)
		if candidate != nil && candidate.Id() == node.Id() {
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
