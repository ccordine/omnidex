package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javascriptgrammar "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

func validateDirectCodingJavaScriptAcceptance(ref assemblyline.SourceBlockRef, declaration string) error {
	if ref.Block.Role != assemblyline.SourceBlockTaskVerification || ref.Block.TaskID == "" {
		return fmt.Errorf("JavaScript acceptance requires one owned verification declaration")
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(javascriptgrammar.Language())); err != nil {
		return err
	}
	source := []byte(declaration)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("JavaScript acceptance parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() || root.NamedChildCount() != 1 || root.NamedChild(0).Kind() != "function_declaration" {
		return fmt.Errorf("JavaScript acceptance requires its exact function declaration")
	}
	body := root.NamedChild(0).ChildByFieldName("body")
	if body == nil || strings.TrimSpace(declaration[:body.StartByte()]) != ref.Block.Signature {
		return fmt.Errorf("JavaScript acceptance changed its code-owned declaration")
	}
	statements := javaScriptAcceptanceChildren(body)
	if len(statements) < 2 {
		return fmt.Errorf("JavaScript acceptance must run the behavior and assert its observed result")
	}
	resultName, err := javaScriptAcceptanceResultBinding(statements[0], source)
	if err != nil {
		return err
	}
	for _, statement := range statements[1:] {
		if !javaScriptAcceptanceAssertion(statement, source, resultName) {
			return fmt.Errorf("JavaScript acceptance requires direct strict assertions against independent expected values")
		}
	}
	return nil
}

func javaScriptAcceptanceResultBinding(statement *treesitter.Node, source []byte) (string, error) {
	invalid := fmt.Errorf("JavaScript acceptance must first bind one literal-input run to its observed result")
	children := javaScriptAcceptanceChildren(statement)
	if (statement.Kind() != "lexical_declaration" && statement.Kind() != "variable_declaration") || len(children) != 1 {
		return "", invalid
	}
	name, value := children[0].ChildByFieldName("name"), children[0].ChildByFieldName("value")
	if name == nil || name.Kind() != "identifier" || value == nil || value.Kind() != "call_expression" || javaScriptAcceptanceOptional(value) {
		return "", invalid
	}
	resultName := name.Utf8Text(source)
	function := value.ChildByFieldName("function")
	arguments := javaScriptAcceptanceChildren(value.ChildByFieldName("arguments"))
	if resultName == "run" || resultName == "assert" || function == nil || function.Kind() != "identifier" || function.Utf8Text(source) != "run" || len(arguments) != 2 {
		return "", invalid
	}
	if !javaScriptAcceptanceInput(arguments[0], source) || arguments[1].Kind() != "object" || !javaScriptAcceptanceLiteral(arguments[1], source) {
		return "", invalid
	}
	return resultName, nil
}

func javaScriptAcceptanceAssertion(statement *treesitter.Node, source []byte, resultName string) bool {
	children := javaScriptAcceptanceChildren(statement)
	if statement.Kind() != "expression_statement" || len(children) != 1 || children[0].Kind() != "call_expression" || javaScriptAcceptanceOptional(children[0]) {
		return false
	}
	call := children[0]
	function := call.ChildByFieldName("function")
	if function == nil {
		return false
	}
	name := function.Utf8Text(source)
	if name != "assert.strictEqual" && name != "assert.deepStrictEqual" {
		return false
	}
	arguments := javaScriptAcceptanceChildren(call.ChildByFieldName("arguments"))
	if len(arguments) < 2 || len(arguments) > 3 || !javaScriptAcceptanceObservedValue(arguments[0], source, resultName) || !javaScriptAcceptanceLiteral(arguments[1], source) {
		return false
	}
	return len(arguments) == 2 || arguments[2].Kind() == "string"
}

func javaScriptAcceptanceObservedValue(node *treesitter.Node, source []byte, name string) bool {
	if node == nil || javaScriptAcceptanceOptional(node) {
		return false
	}
	switch node.Kind() {
	case "identifier":
		return node.Utf8Text(source) == name
	case "member_expression":
		property := node.ChildByFieldName("property")
		return property != nil && property.Kind() == "property_identifier" && javaScriptAcceptanceObservedValue(node.ChildByFieldName("object"), source, name)
	case "subscript_expression":
		index := node.ChildByFieldName("index")
		return index != nil && (index.Kind() == "string" || index.Kind() == "number") && javaScriptAcceptanceObservedValue(node.ChildByFieldName("object"), source, name)
	}
	return false
}

func javaScriptAcceptanceChildren(node *treesitter.Node) []*treesitter.Node {
	var children []*treesitter.Node
	if node != nil {
		for index := uint(0); index < node.NamedChildCount(); index++ {
			child := node.NamedChild(index)
			if child.Kind() != "comment" {
				children = append(children, child)
			}
		}
	}
	return children
}

func javaScriptAcceptanceOptional(node *treesitter.Node) bool {
	for index := uint(0); index < node.ChildCount(); index++ {
		if node.Child(index).Kind() == "optional_chain" {
			return true
		}
	}
	return false
}
