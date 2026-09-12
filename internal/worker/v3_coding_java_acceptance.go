package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javagrammar "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func validateDirectCodingJavaAcceptance(ref assemblyline.SourceBlockRef, declaration string) error {
	if ref.Block.Role != assemblyline.SourceBlockTaskVerification || ref.Block.TaskID == "" {
		return fmt.Errorf("Java acceptance requires one owned verification declaration")
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javagrammar.Language())); err != nil {
		return err
	}
	source := []byte(declaration)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("Java acceptance parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	children := javaNamedSyntaxChildren(root)
	if root.HasError() || len(children) != 1 || children[0].Kind() != "method_declaration" {
		return fmt.Errorf("Java acceptance requires its exact method declaration")
	}
	body := children[0].ChildByFieldName("body")
	if body == nil || strings.TrimSpace(declaration[:body.StartByte()]) != ref.Block.Signature {
		return fmt.Errorf("Java acceptance changed its code-owned declaration")
	}
	statements := javaNamedSyntaxChildren(body)
	if len(statements) < 2 {
		return fmt.Errorf("Java acceptance must run the behavior and assert its observed result")
	}
	resultName, err := javaAcceptanceResultBinding(statements[0], source)
	if err != nil {
		return err
	}
	for _, statement := range statements[1:] {
		operands := javaNamedSyntaxChildren(statement)
		if statement.Kind() != "assert_statement" || len(operands) < 1 || len(operands) > 2 || !javaAcceptanceComparison(operands[0], source, resultName) || len(operands) == 2 && !javaAcceptanceLiteral(operands[1], source) {
			return fmt.Errorf("Java acceptance requires direct assertions against independent expected values")
		}
	}
	return nil
}

func javaAcceptanceResultBinding(statement *treesitter.Node, source []byte) (string, error) {
	invalid := fmt.Errorf("Java acceptance must first bind one literal-input run to its observed result")
	if statement.Kind() != "local_variable_declaration" {
		return "", invalid
	}
	var declarators []*treesitter.Node
	for _, child := range javaNamedSyntaxChildren(statement) {
		if child.Kind() == "variable_declarator" {
			declarators = append(declarators, child)
		}
	}
	if len(declarators) != 1 {
		return "", invalid
	}
	name, value := declarators[0].ChildByFieldName("name"), declarators[0].ChildByFieldName("value")
	if name == nil || name.Kind() != "identifier" || value == nil || value.Kind() != "method_invocation" {
		return "", invalid
	}
	method := value.ChildByFieldName("name")
	arguments := javaNamedSyntaxChildren(value.ChildByFieldName("arguments"))
	if method == nil || method.Utf8Text(source) != "run" || value.ChildByFieldName("object") != nil || len(arguments) != 2 || !javaAcceptanceInput(arguments[0], source) || !javaAcceptanceLiteral(arguments[1], source) {
		return "", invalid
	}
	if _, ok := javaAcceptanceFactoryArguments(arguments[1], source, "Map", "of"); !ok {
		return "", invalid
	}
	return name.Utf8Text(source), nil
}

func javaAcceptanceComparison(node *treesitter.Node, source []byte, resultName string) bool {
	if node == nil {
		return false
	}
	if node.Kind() == "parenthesized_expression" {
		children := javaNamedSyntaxChildren(node)
		return len(children) == 1 && javaAcceptanceComparison(children[0], source, resultName)
	}
	if node.Kind() == "binary_expression" {
		operator := node.ChildByFieldName("operator")
		left, right := node.ChildByFieldName("left"), node.ChildByFieldName("right")
		if operator == nil {
			return false
		}
		switch operator.Utf8Text(source) {
		case "&&":
			return javaAcceptanceComparison(left, source, resultName) && javaAcceptanceComparison(right, source, resultName)
		case "==":
			return javaAcceptanceObservedValue(left, source, resultName) && javaAcceptanceScalarLiteral(right, source) || javaAcceptanceScalarLiteral(left, source) && javaAcceptanceObservedValue(right, source, resultName)
		}
	}
	if node.Kind() != "method_invocation" {
		return false
	}
	name, object := node.ChildByFieldName("name"), node.ChildByFieldName("object")
	arguments := javaNamedSyntaxChildren(node.ChildByFieldName("arguments"))
	if name == nil || name.Utf8Text(source) != "equals" || len(arguments) != 1 {
		return false
	}
	return javaAcceptanceObservedValue(object, source, resultName) && javaAcceptanceLiteral(arguments[0], source) || javaAcceptanceLiteral(object, source) && javaAcceptanceObservedValue(arguments[0], source, resultName)
}

func javaAcceptanceObservedValue(node *treesitter.Node, source []byte, resultName string) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier":
		return node.Utf8Text(source) == resultName
	case "parenthesized_expression":
		children := javaNamedSyntaxChildren(node)
		return len(children) == 1 && javaAcceptanceObservedValue(children[0], source, resultName)
	case "cast_expression":
		return javaAcceptanceObservedValue(node.ChildByFieldName("value"), source, resultName)
	case "method_invocation":
		name := node.ChildByFieldName("name")
		if name == nil || !javaAcceptanceObservedValue(node.ChildByFieldName("object"), source, resultName) {
			return false
		}
		arguments := javaNamedSyntaxChildren(node.ChildByFieldName("arguments"))
		switch name.Utf8Text(source) {
		case "get":
			return len(arguments) == 1 && javaAcceptanceLiteral(arguments[0], source)
		case "size", "length", "isEmpty", "intValue", "longValue", "doubleValue":
			return len(arguments) == 0
		}
	}
	return false
}
