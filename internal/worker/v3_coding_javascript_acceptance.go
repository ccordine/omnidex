package worker

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	javascriptgrammar "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

func validateDirectCodingJavaScriptAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	implementationName, err := directCodingAcceptanceImplementationName(stage, ref, "function ")
	if err != nil {
		return fmt.Errorf("JavaScript acceptance block %s: %w", ref.Block.ID, err)
	}
	requiredAssertions, err := directCodingAcceptanceObligationCount(stage, ref)
	if err != nil {
		return err
	}
	called, assertions, err := inspectJavaScriptAcceptance(
		[]byte(source), javascriptgrammar.Language(), implementationName,
	)
	if err != nil {
		return err
	}
	if !called || assertions < requiredAssertions {
		return fmt.Errorf(
			"JavaScript acceptance block %s must bind one %s result and prove the accepted requirement with %d direct result-field assertion",
			ref.Block.ID, implementationName, requiredAssertions,
		)
	}
	return nil
}

func directCodingAcceptanceImplementationName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	prefix string,
) (string, error) {
	name := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		if name != "" {
			return "", fmt.Errorf("multiple implementation owners")
		}
		signature := strings.TrimPrefix(dependency.Signature, prefix)
		separator := strings.IndexByte(signature, '(')
		if separator <= 0 {
			return "", fmt.Errorf("implementation signature %q has no callable name", dependency.Signature)
		}
		name = strings.TrimSpace(signature[:separator])
	}
	if name == "" {
		return "", fmt.Errorf("no implementation owner")
	}
	return name, nil
}

func inspectJavaScriptAcceptance(
	source []byte,
	language unsafe.Pointer,
	implementationName string,
) (bool, int, error) {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		return false, 0, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return false, 0, fmt.Errorf("JavaScript acceptance parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return false, 0, fmt.Errorf("JavaScript acceptance source is not parseable")
	}
	functionBody, resultNames, calls, err := inspectJavaScriptAcceptanceAuthority(
		source, root, implementationName,
	)
	if err != nil {
		return false, 0, err
	}
	if calls != 1 || len(resultNames) != 1 {
		return false, 0, nil
	}
	assertions := make(map[string]struct{})
	for index := uint(0); index < functionBody.NamedChildCount(); index++ {
		statement := functionBody.NamedChild(index)
		switch statement.Kind() {
		case "lexical_declaration", "variable_declaration":
			continue
		case "expression_statement":
			if statement.NamedChildCount() != 1 {
				return false, 0, nil
			}
			key, valid := javaScriptAcceptanceAssertion(
				source, statement.NamedChild(0), resultNames,
			)
			if !valid {
				return false, 0, nil
			}
			assertions[key] = struct{}{}
		default:
			return false, 0, nil
		}
	}
	return true, len(assertions), nil
}

func inspectJavaScriptAcceptanceAuthority(
	source []byte,
	root *treesitter.Node,
	implementationName string,
) (*treesitter.Node, map[string]struct{}, int, error) {
	var functionBody *treesitter.Node
	resultNames := make(map[string]struct{})
	functionCount := 0
	implementationCalls := 0
	forbidden := ""
	var visit func(*treesitter.Node)
	visit = func(node *treesitter.Node) {
		if node.Kind() == "function_declaration" {
			functionCount++
			functionBody = node.ChildByFieldName("body")
		}
		if node.Kind() == "identifier" {
			value := javaScriptNodeText(source, node)
			switch value {
			case "process", "require", "fetch", "eval", "Function", "WebSocket":
				forbidden = value
			}
		}
		if node.Kind() == "variable_declarator" {
			name := node.ChildByFieldName("name")
			value := node.ChildByFieldName("value")
			if name != nil {
				declared := javaScriptNodeText(source, name)
				if declared == "assert" || declared == implementationName {
					forbidden = "redefined " + declared
				}
				if functionBody != nil && javaScriptAcceptanceDirectDeclaration(node, functionBody) &&
					javaScriptAcceptanceExactCall(source, value, implementationName) {
					resultNames[declared] = struct{}{}
				}
			}
		}
		if node.Kind() == "call_expression" {
			callee := node.ChildByFieldName("function")
			if callee != nil && javaScriptNodeText(source, callee) == implementationName {
				implementationCalls++
			}
		}
		for index := uint(0); index < node.ChildCount(); index++ {
			visit(node.Child(index))
		}
	}
	visit(root)
	if forbidden != "" {
		return nil, nil, 0, fmt.Errorf("JavaScript acceptance uses forbidden authority %s", forbidden)
	}
	if functionCount != 1 || functionBody == nil {
		return nil, nil, 0, fmt.Errorf("JavaScript acceptance contains %d function declarations", functionCount)
	}
	return functionBody, resultNames, implementationCalls, nil
}

func javaScriptAcceptanceDirectDeclaration(node, body *treesitter.Node) bool {
	return node != nil && body != nil && node.Parent() != nil && node.Parent().Parent() != nil &&
		node.Parent().Parent().Id() == body.Id()
}

func javaScriptAcceptanceExactCall(
	source []byte,
	node *treesitter.Node,
	implementationName string,
) bool {
	if node == nil || node.Kind() != "call_expression" {
		return false
	}
	callee := node.ChildByFieldName("function")
	return callee != nil && javaScriptNodeText(source, callee) == implementationName
}
