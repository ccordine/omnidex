package worker

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func directCodingBrowserPublicRenderRoot(
	root *treesitter.Node,
) (*treesitter.Node, error) {
	functions, jsxNodes, err := directCodingBrowserBoundedRootNodes(root)
	if err != nil {
		return nil, err
	}
	if len(functions) != 1 {
		return nil, fmt.Errorf(
			"browser public surface requires exactly one function declaration",
		)
	}
	body := functions[0].ChildByFieldName("body")
	if body == nil || body.Kind() != "statement_block" {
		return nil, fmt.Errorf("browser public surface function has no statement body")
	}
	returns := make([]*treesitter.Node, 0, 1)
	for index := uint(0); index < body.NamedChildCount(); index++ {
		child := body.NamedChild(index)
		if child != nil && child.Kind() == "return_statement" {
			returns = append(returns, child)
		}
	}
	if len(returns) != 1 {
		return nil, fmt.Errorf(
			"browser public surface requires one unconditional top-level return",
		)
	}
	returned := returns[0]
	for _, jsx := range jsxNodes {
		if !directCodingBrowserNodeInside(jsx, body) {
			continue
		}
		if !directCodingBrowserNodeInside(jsx, returned) {
			return nil, fmt.Errorf(
				"browser public surface rejects JSX outside the unconditional return",
			)
		}
	}
	scopeReturns, scopeThrows := directCodingBrowserRenderControlFlow(body)
	if len(scopeThrows) != 0 {
		return nil, fmt.Errorf(
			"browser public surface rejects throw in render function control flow",
		)
	}
	if len(scopeReturns) != 1 || scopeReturns[0].Id() != returned.Id() {
		return nil, fmt.Errorf(
			"browser public surface rejects return outside the unconditional top-level return",
		)
	}
	if returned.NamedChildCount() != 1 {
		return nil, fmt.Errorf("browser public surface return must contain one JSX root")
	}
	render := returned.NamedChild(0)
	for render != nil && render.Kind() == "parenthesized_expression" {
		if render.NamedChildCount() != 1 {
			return nil, fmt.Errorf("browser public surface return has an ambiguous expression")
		}
		render = render.NamedChild(0)
	}
	if render == nil || (render.Kind() != "jsx_element" &&
		render.Kind() != "jsx_self_closing_element") {
		return nil, fmt.Errorf(
			"browser public surface return must be one unconditional intrinsic JSX root",
		)
	}
	return render, nil
}

func directCodingBrowserRenderControlFlow(
	body *treesitter.Node,
) ([]*treesitter.Node, []*treesitter.Node) {
	returns := make([]*treesitter.Node, 0, 1)
	throws := make([]*treesitter.Node, 0)
	stack := make([]*treesitter.Node, 0, body.NamedChildCount())
	for index := body.NamedChildCount(); index > 0; index-- {
		stack = append(stack, body.NamedChild(index-1))
	}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil || javaScriptFunctionScopeKind(node.Kind()) {
			continue
		}
		switch node.Kind() {
		case "return_statement":
			returns = append(returns, node)
		case "throw_statement":
			throws = append(throws, node)
		}
		for index := node.NamedChildCount(); index > 0; index-- {
			stack = append(stack, node.NamedChild(index-1))
		}
	}
	return returns, throws
}

func directCodingBrowserBoundedRootNodes(
	root *treesitter.Node,
) ([]*treesitter.Node, []*treesitter.Node, error) {
	functions := make([]*treesitter.Node, 0, 1)
	jsxNodes := make([]*treesitter.Node, 0)
	stack := []*treesitter.Node{root}
	visited := 0
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}
		visited++
		if visited > directCodingBrowserPublicSurfaceMaxNodes {
			return nil, nil, fmt.Errorf(
				"browser public surface exceeds %d syntax nodes",
				directCodingBrowserPublicSurfaceMaxNodes,
			)
		}
		switch node.Kind() {
		case "function_declaration":
			functions = append(functions, node)
		case "jsx_element", "jsx_self_closing_element":
			jsxNodes = append(jsxNodes, node)
		}
		for index := node.NamedChildCount(); index > 0; index-- {
			stack = append(stack, node.NamedChild(index-1))
		}
	}
	return functions, jsxNodes, nil
}

func directCodingBrowserNodeInside(
	node *treesitter.Node,
	owner *treesitter.Node,
) bool {
	return node != nil && owner != nil && node.StartByte() >= owner.StartByte() &&
		node.EndByte() <= owner.EndByte()
}
