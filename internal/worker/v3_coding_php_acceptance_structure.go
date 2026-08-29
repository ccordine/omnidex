package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func phpAcceptanceDirectFunctionBody(
	root *treesitter.Node,
	source []byte,
) (*treesitter.Node, error) {
	if root == nil || root.NamedChildCount() != 1 {
		return nil, fmt.Errorf("PHP acceptance requires one verifier function")
	}
	function := root.NamedChild(0)
	if function == nil || function.Kind() != "function_definition" {
		return nil, fmt.Errorf("PHP acceptance root is not a verifier function")
	}
	body := function.ChildByFieldName("body")
	if body == nil || body.Kind() != "compound_statement" {
		return nil, fmt.Errorf("PHP acceptance verifier has no direct function body")
	}
	bindings := 0
	directStatements := 0
	for index := uint(0); index < body.NamedChildCount(); index++ {
		statement := body.NamedChild(index)
		if statement == nil || statement.Kind() == "comment" {
			continue
		}
		directStatements++
		if statement.Kind() != "expression_statement" || statement.NamedChildCount() != 1 {
			return nil, fmt.Errorf(
				"PHP acceptance verifier contains nested or non-direct statement %s", statement.Kind(),
			)
		}
		expression := statement.NamedChild(0)
		switch expression.Kind() {
		case "assignment_expression":
			bindings++
			if bindings != 1 || directStatements != 1 {
				return nil, fmt.Errorf(
					"PHP acceptance permits only the first exact feature-result binding",
				)
			}
			left := expression.ChildByFieldName("left")
			if left == nil || left.Kind() != "variable_name" {
				return nil, fmt.Errorf("PHP acceptance setup must bind one local variable")
			}
			var nestedAssignment bool
			walkPHPTree(expression.ChildByFieldName("right"), func(candidate *treesitter.Node) {
				if candidate.Kind() == "assignment_expression" ||
					candidate.Kind() == "reference_assignment_expression" {
					nestedAssignment = true
				}
			})
			if nestedAssignment {
				return nil, fmt.Errorf("PHP acceptance setup contains a nested assignment")
			}
		case "scoped_call_expression":
			scope, method := expression.ChildByFieldName("scope"), expression.ChildByFieldName("name")
			if phpNodeText(source, scope) != "RuntimeAssertions" || method == nil {
				return nil, fmt.Errorf("PHP acceptance direct call is not a code-owned assertion")
			}
			switch phpNodeText(source, method) {
			case "requireResult", "require":
			default:
				return nil, fmt.Errorf("PHP acceptance direct assertion is not registered")
			}
		default:
			return nil, fmt.Errorf(
				"PHP acceptance verifier contains non-binding expression %s", expression.Kind(),
			)
		}
	}
	if bindings != 1 {
		return nil, fmt.Errorf("PHP acceptance requires one exact feature-result binding")
	}
	return body, nil
}

func phpAcceptanceCallIsDirectStatement(
	call *treesitter.Node,
	body *treesitter.Node,
) bool {
	if call == nil || body == nil || call.Parent() == nil || call.Parent().Parent() == nil {
		return false
	}
	return call.Parent().Kind() == "expression_statement" &&
		call.Parent().Parent().Id() == body.Id()
}
