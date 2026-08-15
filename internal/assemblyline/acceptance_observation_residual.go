package assemblyline

import treesitter "github.com/tree-sitter/go-tree-sitter"

func acceptanceResidualStructure(
	node *treesitter.Node,
	source []byte,
) ([]string, []string) {
	structure := []string{}
	operators := []string{}
	walkAcceptanceResidual(node, source, func(current *treesitter.Node) {
		if !acceptanceIdentifierKind(current.Kind()) {
			structure = append(structure, current.Kind())
		}
		if operator := acceptanceOperator(current, source); operator != "" {
			operators = append(operators, operator)
		}
	})
	return structure, operators
}

// walkAcceptanceResidual visits statement structure that is not already owned
// by an independently inventoried call site. Registered harness calls expose
// their direct non-call arguments so callback control flow cannot hide inside
// waitFor, while nested calls remain owned by their own atomic sites.
func walkAcceptanceResidual(
	node *treesitter.Node,
	source []byte,
	visit func(*treesitter.Node),
) {
	if node == nil || node.Kind() == "comment" {
		return
	}
	if node.Kind() == "call_expression" {
		if acceptancePlatformOperation(acceptanceCallOperation(node, source)) {
			walkAcceptanceHarnessArguments(node.ChildByFieldName("arguments"), visit)
		}
		return
	}
	visit(node)
	for index := uint(0); index < node.NamedChildCount(); index++ {
		walkAcceptanceResidual(node.NamedChild(index), source, visit)
	}
}

func walkAcceptanceHarnessArguments(
	node *treesitter.Node,
	visit func(*treesitter.Node),
) {
	if node == nil || node.Kind() == "comment" || node.Kind() == "call_expression" {
		return
	}
	visit(node)
	for index := uint(0); index < node.NamedChildCount(); index++ {
		walkAcceptanceHarnessArguments(node.NamedChild(index), visit)
	}
}
