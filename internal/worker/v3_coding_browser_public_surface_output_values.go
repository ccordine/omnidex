package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// outputValueChildren projects only syntax children whose
// values can become the enclosing expression's value. JavaScript still
// evaluates discarded sequence operands and void operands, but their values
// cannot establish output provenance.
func (flow directCodingBrowserOutputDataflow) outputValueChildren(
	node *treesitter.Node,
) ([]*treesitter.Node, bool) {
	if node == nil {
		return nil, false
	}
	if node.Kind() == "call_expression" {
		children, known := flow.projectBuiltinOutputCall(node)
		if !known {
			return nil, true
		}
		return children, true
	}
	if selected, projected := flow.projectMemberValue(node); projected {
		if selected == nil {
			return nil, true
		}
		return []*treesitter.Node{selected}, true
	}
	switch node.Kind() {
	case "sequence_expression":
		count := node.NamedChildCount()
		if count == 0 {
			return nil, true
		}
		return []*treesitter.Node{node.NamedChild(count - 1)}, true
	case "unary_expression":
		argument := node.ChildByFieldName("argument")
		if argument == nil || argument.StartByte() < node.StartByte() ||
			argument.StartByte() > uint(len(flow.source)) {
			return nil, false
		}
		operator := strings.TrimSpace(string(flow.source[node.StartByte():argument.StartByte()]))
		if strings.HasPrefix(operator, "void") {
			return nil, true
		}
	case "assignment_expression":
		value := node.ChildByFieldName("right")
		if value == nil {
			return nil, true
		}
		return []*treesitter.Node{value}, true
	}
	return nil, false
}
