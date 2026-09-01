package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserRuntimeAuthoritativeValues map[uintptr]struct{}

type directCodingBrowserRuntimeAuthoritativeCandidate struct {
	declarationIDs []uintptr
	value          *treesitter.Node
}

func collectDirectCodingBrowserRuntimeAuthoritativeValues(
	root *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) directCodingBrowserRuntimeAuthoritativeValues {
	values := make(directCodingBrowserRuntimeAuthoritativeValues)
	function := directCodingBrowserRuntimePublicFunction(root)
	if function == nil {
		return values
	}
	for _, binding := range directCodingBrowserOutputPatternBindings(
		function.ChildByFieldName("parameters"),
	) {
		name := directCodingBrowserRuntimeNodeText(source, binding)
		if name == "state" || name == "capabilities" {
			values[binding.Id()] = struct{}{}
		}
	}
	candidates := collectDirectCodingBrowserRuntimeAuthoritativeCandidates(
		function, source, bindings,
	)
	for changed := true; changed; {
		changed = false
		for _, candidate := range candidates {
			if !values.expressionMayBeAuthoritative(candidate.value, source, bindings) {
				continue
			}
			for _, declarationID := range candidate.declarationIDs {
				if _, exists := values[declarationID]; !exists {
					values[declarationID] = struct{}{}
					changed = true
				}
			}
		}
	}
	return values
}

func collectDirectCodingBrowserRuntimeAuthoritativeCandidates(
	root *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) []directCodingBrowserRuntimeAuthoritativeCandidate {
	candidates := make([]directCodingBrowserRuntimeAuthoritativeCandidate, 0)
	var walk func(*treesitter.Node)
	walk = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "variable_declarator":
			ids := directCodingBrowserRuntimePatternDeclarationIDs(
				node.ChildByFieldName("name"), source, bindings, true,
			)
			if len(ids) != 0 {
				candidates = append(candidates, directCodingBrowserRuntimeAuthoritativeCandidate{
					declarationIDs: ids, value: node.ChildByFieldName("value"),
				})
			}
		case "assignment_expression":
			left := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("left"))
			if left == nil || left.Kind() == "member_expression" ||
				left.Kind() == "subscript_expression" {
				break
			}
			ids := directCodingBrowserRuntimePatternDeclarationIDs(
				left, source, bindings, false,
			)
			if len(ids) != 0 {
				candidates = append(candidates, directCodingBrowserRuntimeAuthoritativeCandidate{
					declarationIDs: ids, value: node.ChildByFieldName("right"),
				})
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			walk(node.NamedChild(index))
		}
	}
	walk(root)
	return candidates
}

func directCodingBrowserRuntimePatternDeclarationIDs(
	pattern *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
	declaration bool,
) []uintptr {
	ids := make([]uintptr, 0, 2)
	for _, bindingNode := range directCodingBrowserOutputPatternBindings(pattern) {
		if declaration {
			ids = append(ids, bindingNode.Id())
			continue
		}
		binding, resolved := bindings.resolve(
			directCodingBrowserRuntimeNodeText(source, bindingNode), bindingNode,
		)
		if resolved {
			ids = append(ids, binding.declarationID)
		}
	}
	return ids
}

func (values directCodingBrowserRuntimeAuthoritativeValues) reference(
	node *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil || node.Kind() != "identifier" {
		return false
	}
	binding, resolved := bindings.resolve(
		directCodingBrowserRuntimeNodeText(source, node), node,
	)
	if !resolved {
		return false
	}
	_, authoritative := values[binding.declarationID]
	return authoritative
}

func (values directCodingBrowserRuntimeAuthoritativeValues) expressionMayBeAuthoritative(
	node *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "identifier":
		return values.reference(node, source, bindings)
	case "member_expression", "subscript_expression":
		return values.expressionMayBeAuthoritative(
			node.ChildByFieldName("object"), source, bindings,
		)
	case "sequence_expression":
		if node.NamedChildCount() == 0 {
			return false
		}
		return values.expressionMayBeAuthoritative(
			node.NamedChild(node.NamedChildCount()-1), source, bindings,
		)
	case "assignment_expression":
		return values.expressionMayBeAuthoritative(
			node.ChildByFieldName("right"), source, bindings,
		)
	case "ternary_expression":
		return values.expressionMayBeAuthoritative(
			node.ChildByFieldName("consequence"), source, bindings,
		) || values.expressionMayBeAuthoritative(
			node.ChildByFieldName("alternative"), source, bindings,
		)
	case "binary_expression":
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		if left == nil || right == nil || left.EndByte() > right.StartByte() ||
			right.StartByte() > uint(len(source)) {
			return false
		}
		operator := strings.TrimSpace(string(source[left.EndByte():right.StartByte()]))
		if operator != "&&" && operator != "||" && operator != "??" {
			return false
		}
		return values.expressionMayBeAuthoritative(
			node.ChildByFieldName("left"), source, bindings,
		) || values.expressionMayBeAuthoritative(
			node.ChildByFieldName("right"), source, bindings,
		)
	case "await_expression":
		return values.expressionMayBeAuthoritative(
			node.ChildByFieldName("argument"), source, bindings,
		)
	default:
		return false
	}
}

func directCodingBrowserRuntimePublicFunction(root *treesitter.Node) *treesitter.Node {
	var function *treesitter.Node
	var walk func(*treesitter.Node)
	walk = func(node *treesitter.Node) {
		if node == nil || function != nil {
			return
		}
		if node.Kind() == "function_declaration" && !directCodingBrowserFunctionIsNested(node) {
			function = node
			return
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			walk(node.NamedChild(index))
		}
	}
	walk(root)
	return function
}
