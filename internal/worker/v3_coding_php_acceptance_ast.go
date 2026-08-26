package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	phpgrammar "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

type phpAcceptanceInspection struct {
	Called         bool
	ShapeChecked   bool
	ConditionCount int
}

func inspectPHPAcceptance(
	source []byte,
	featureName string,
	fixtureName string,
	requiredConditions int,
) (phpAcceptanceInspection, error) {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(phpgrammar.LanguagePHPOnly())); err != nil {
		return phpAcceptanceInspection{}, err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return phpAcceptanceInspection{}, fmt.Errorf("PHP acceptance parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return phpAcceptanceInspection{}, fmt.Errorf("PHP acceptance source is not parseable")
	}
	body, err := phpAcceptanceDirectFunctionBody(root, source)
	if err != nil {
		return phpAcceptanceInspection{}, err
	}
	resultNames := make(map[string]struct{})
	assignmentCounts := make(map[string]int)
	featureCalls := 0
	walkPHPTree(root, func(node *treesitter.Node) {
		if phpExactFunctionCall(source, node, featureName) {
			featureCalls++
		}
		if node.Kind() != "assignment_expression" {
			return
		}
		left, right := node.ChildByFieldName("left"), node.ChildByFieldName("right")
		if left == nil || left.Kind() != "variable_name" {
			return
		}
		name := phpNodeText(source, left)
		assignmentCounts[name]++
		if phpExactFixtureFeatureCall(source, right, featureName, fixtureName) {
			resultNames[name] = struct{}{}
		}
	})
	inspection := phpAcceptanceInspection{Called: featureCalls > 0}
	if featureCalls != 1 || len(resultNames) != 1 {
		return inspection, nil
	}
	resultName := ""
	for name := range resultNames {
		resultName = name
	}
	if assignmentCounts[resultName] != 1 {
		return inspection, nil
	}
	shapeChecks := 0
	conditions := make(map[string]struct{}, requiredConditions)
	var assertionFailure error
	walkPHPTree(root, func(node *treesitter.Node) {
		if assertionFailure != nil || node.Kind() != "scoped_call_expression" {
			return
		}
		scope, method := node.ChildByFieldName("scope"), node.ChildByFieldName("name")
		if phpNodeText(source, scope) != "RuntimeAssertions" || method == nil {
			return
		}
		if !phpAcceptanceCallIsDirectStatement(node, body) {
			assertionFailure = fmt.Errorf("PHP acceptance assertion is not a direct verifier statement")
			return
		}
		arguments := phpCallArguments(node.ChildByFieldName("arguments"))
		methodName := phpNodeText(source, method)
		expectedArguments := 1
		if methodName == "require" {
			expectedArguments = 3
		}
		if len(arguments) != expectedArguments || len(arguments) == 0 ||
			arguments[0].Kind() != "variable_name" || phpNodeText(source, arguments[0]) != resultName {
			assertionFailure = fmt.Errorf(
				"RuntimeAssertions::%s is detached from the exact feature result", methodName,
			)
			return
		}
		switch methodName {
		case "requireResult":
			shapeChecks++
		case "require":
			fingerprint, err := phpAcceptanceConditionFingerprint(source, arguments[1], resultName)
			if err != nil {
				assertionFailure = err
				return
			}
			if _, duplicate := conditions[fingerprint]; duplicate {
				assertionFailure = fmt.Errorf("PHP acceptance repeats a result condition")
				return
			}
			conditions[fingerprint] = struct{}{}
		default:
			assertionFailure = fmt.Errorf("PHP acceptance uses unknown RuntimeAssertions method %s", methodName)
		}
	})
	if assertionFailure != nil {
		return inspection, assertionFailure
	}
	inspection.ShapeChecked = shapeChecks == 1
	inspection.ConditionCount = len(conditions)
	return inspection, nil
}

func phpExactFixtureFeatureCall(
	source []byte,
	node *treesitter.Node,
	featureName string,
	fixtureName string,
) bool {
	if !phpExactFunctionCall(source, node, featureName) {
		return false
	}
	arguments := phpCallArguments(node.ChildByFieldName("arguments"))
	return len(arguments) == 2 && phpExactZeroArgumentCall(source, arguments[0], fixtureName)
}

func phpExactFunctionCall(source []byte, node *treesitter.Node, name string) bool {
	if node == nil || node.Kind() != "function_call_expression" {
		return false
	}
	function := node.ChildByFieldName("function")
	return function != nil && function.Kind() == "name" && phpNodeText(source, function) == name
}

func phpExactZeroArgumentCall(source []byte, node *treesitter.Node, name string) bool {
	return phpExactFunctionCall(source, node, name) &&
		len(phpCallArguments(node.ChildByFieldName("arguments"))) == 0
}

func phpCallArguments(arguments *treesitter.Node) []*treesitter.Node {
	if arguments == nil {
		return nil
	}
	values := make([]*treesitter.Node, 0, arguments.NamedChildCount())
	for index := uint(0); index < arguments.NamedChildCount(); index++ {
		child := arguments.NamedChild(index)
		if child == nil {
			continue
		}
		if child.Kind() == "argument" && child.NamedChildCount() > 0 {
			child = child.NamedChild(child.NamedChildCount() - 1)
		}
		if child != nil {
			values = append(values, child)
		}
	}
	return values
}
