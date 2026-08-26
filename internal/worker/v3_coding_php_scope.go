package worker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	phpgrammar "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

var phpTopLevelAPIFunction = regexp.MustCompile(`(?m)^function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var phpAPIClass = regexp.MustCompile(`(?m)^(?:final\s+(?:readonly\s+)?)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
var phpAPIStaticMethod = regexp.MustCompile(`(?m)^\s+public\s+static\s+function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

func validateDirectCodingPHPFragment(
	input assemblyline.FragmentGenerationInput,
	candidate string,
) (string, error) {
	validated, err := assemblyline.ValidatePHPFragment(input.Signature, candidate)
	if err != nil {
		return "", err
	}
	if err := validatePHPFragmentAuthority(input, []byte(validated)); err != nil {
		return "", err
	}
	if phpHTMLRendererSignature.MatchString(input.Signature) {
		if err := validatePHPHTMLRenderer([]byte(validated)); err != nil {
			return "", err
		}
	}
	return validated, nil
}

func validatePHPFragmentAuthority(
	input assemblyline.FragmentGenerationInput,
	source []byte,
) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(phpgrammar.LanguagePHPOnly())); err != nil {
		return err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("PHP authority parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("PHP authority parser rejected the declaration")
	}
	allowedFunctions := phpSafeFunctions()
	allowedClasses := make(map[string]struct{})
	allowedStaticMethods := make(map[string]struct{})
	for _, api := range append(append([]string(nil), input.Capabilities...), input.PermittedSymbols...) {
		for _, match := range phpTopLevelAPIFunction.FindAllStringSubmatch(api, -1) {
			allowedFunctions[strings.ToLower(match[1])] = struct{}{}
		}
		for _, match := range phpAPIClass.FindAllStringSubmatch(api, -1) {
			allowedClasses[strings.ToLower(match[1])] = struct{}{}
		}
		for _, match := range phpAPIStaticMethod.FindAllStringSubmatch(api, -1) {
			allowedStaticMethods[strings.ToLower(match[1])] = struct{}{}
		}
	}
	boundVariables := make(map[string]struct{})
	collectPHPVariableBindings(root, source, boundVariables)
	functionDefinitions := 0
	var authorityFailure error
	walkPHPTree(root, func(node *treesitter.Node) {
		if authorityFailure != nil {
			return
		}
		switch node.Kind() {
		case "function_definition":
			functionDefinitions++
		case "anonymous_function", "arrow_function", "anonymous_class", "class_declaration",
			"namespace_definition", "namespace_use_declaration", "use_declaration",
			"include_expression", "include_once_expression", "require_expression",
			"require_once_expression", "shell_command_expression", "global_declaration",
			"function_static_declaration", "static_variable_declaration", "catch_clause",
			"goto_statement", "exit_statement", "echo_statement", "print_intrinsic",
			"yield_expression", "throw_expression", "attribute_list", "dynamic_variable_name",
			"reference_assignment_expression", "reference_modifier", "declare_statement",
			"heredoc", "nowdoc":
			authorityFailure = fmt.Errorf("PHP fragment uses forbidden authority %s", node.Kind())
		case "variable_name":
			name := phpNodeText(source, node)
			if phpForbiddenVariable(name) {
				authorityFailure = fmt.Errorf("PHP fragment uses forbidden variable %s", name)
				return
			}
			if _, exists := boundVariables[name]; !exists {
				authorityFailure = fmt.Errorf("PHP fragment references undeclared local variable %s", name)
			}
		case "function_call_expression":
			function := node.ChildByFieldName("function")
			if function == nil || function.Kind() != "name" {
				authorityFailure = fmt.Errorf("PHP fragment uses a dynamic or qualified function call")
				return
			}
			name := strings.ToLower(phpNodeText(source, function))
			if _, allowed := allowedFunctions[name]; !allowed {
				authorityFailure = fmt.Errorf("PHP fragment calls undeclared function %s", name)
			}
		case "scoped_call_expression":
			scope, method := node.ChildByFieldName("scope"), node.ChildByFieldName("name")
			if scope == nil || method == nil || scope.Kind() != "name" || method.Kind() != "name" {
				authorityFailure = fmt.Errorf("PHP fragment uses a dynamic static call")
				return
			}
			className := strings.ToLower(phpNodeText(source, scope))
			methodName := strings.ToLower(phpNodeText(source, method))
			if _, allowed := allowedClasses[className]; !allowed {
				authorityFailure = fmt.Errorf("PHP fragment calls undeclared class %s", className)
				return
			}
			if _, allowed := allowedStaticMethods[methodName]; !allowed {
				authorityFailure = fmt.Errorf("PHP fragment calls undeclared static method %s", methodName)
			}
		case "member_call_expression", "nullsafe_member_call_expression":
			authorityFailure = fmt.Errorf("PHP fragment cannot invoke object methods")
		case "object_creation_expression":
			authorityFailure = fmt.Errorf("PHP fragment cannot construct objects")
		case "name":
			name := strings.ToUpper(phpNodeText(source, node))
			if phpForbiddenEnvironmentName(name) {
				authorityFailure = fmt.Errorf("PHP fragment uses forbidden environment symbol %s", name)
			}
		}
	})
	if authorityFailure != nil {
		return authorityFailure
	}
	if functionDefinitions != 1 {
		return fmt.Errorf("PHP fragment contains %d function declarations", functionDefinitions)
	}
	return nil
}

func collectPHPVariableBindings(
	node *treesitter.Node,
	source []byte,
	bound map[string]struct{},
) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "simple_parameter", "variadic_parameter", "property_promotion_parameter":
		collectPHPVariables(node.ChildByFieldName("name"), source, bound)
	case "assignment_expression", "reference_assignment_expression":
		collectPHPAssignmentBindings(node.ChildByFieldName("left"), source, bound)
	case "foreach_statement":
		seenAs := false
		body := node.ChildByFieldName("body")
		for index := uint(0); index < node.ChildCount(); index++ {
			child := node.Child(index)
			if child == nil {
				continue
			}
			if body != nil && child.Id() == body.Id() {
				break
			}
			if child.Kind() == "as" {
				seenAs = true
				continue
			}
			if seenAs {
				collectPHPVariables(child, source, bound)
			}
		}
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		collectPHPVariableBindings(node.Child(index), source, bound)
	}
}

func collectPHPAssignmentBindings(
	node *treesitter.Node,
	source []byte,
	bound map[string]struct{},
) {
	if node == nil {
		return
	}
	if node.Kind() == "variable_name" {
		bound[phpNodeText(source, node)] = struct{}{}
		return
	}
	if node.Kind() == "list_literal" {
		collectPHPVariables(node, source, bound)
	}
}

func collectPHPVariables(node *treesitter.Node, source []byte, bound map[string]struct{}) {
	if node == nil {
		return
	}
	if node.Kind() == "variable_name" {
		bound[phpNodeText(source, node)] = struct{}{}
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		collectPHPVariables(node.Child(index), source, bound)
	}
}

func walkPHPTree(node *treesitter.Node, visit func(*treesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for index := uint(0); index < node.ChildCount(); index++ {
		walkPHPTree(node.Child(index), visit)
	}
}

func phpNodeText(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func phpForbiddenVariable(name string) bool {
	switch strings.ToUpper(name) {
	case "$GLOBALS", "$_SERVER", "$_ENV", "$_GET", "$_POST", "$_REQUEST",
		"$_COOKIE", "$_SESSION", "$_FILES", "$HTTP_RAW_POST_DATA", "$THIS":
		return true
	default:
		return false
	}
}

func phpForbiddenEnvironmentName(name string) bool {
	if strings.HasPrefix(name, "PHP_") ||
		(strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")) {
		return true
	}
	switch name {
	case "STDIN", "STDOUT", "STDERR", "DIRECTORY_SEPARATOR", "PATH_SEPARATOR",
		"DEFAULT_INCLUDE_PATH":
		return true
	default:
		return false
	}
}

func phpSafeFunctions() map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, name := range []string{
		"abs", "array_key_exists", "array_keys", "array_values", "ceil", "count",
		"explode", "floor", "implode", "in_array", "is_array", "is_bool",
		"is_float", "is_int", "is_null", "is_numeric", "is_string", "max", "min",
		"round", "sprintf", "str_contains", "str_ends_with", "str_starts_with",
		"strlen", "strtolower", "strtoupper", "substr", "trim", "ltrim", "rtrim",
	} {
		allowed[name] = struct{}{}
	}
	return allowed
}
