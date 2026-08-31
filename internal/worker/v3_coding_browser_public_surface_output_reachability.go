package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// directCodingBrowserSetterCallPotentiallyReachable rejects only control-flow
// positions whose non-execution is exact from local syntax. Runtime-dependent
// conditions remain potentially reachable and therefore retain no authority
// beyond the ordinary setter-value proof.
func directCodingBrowserSetterCallPotentiallyReachable(
	call *treesitter.Node,
	handler *treesitter.Node,
	source []byte,
) bool {
	if call == nil || handler == nil || !directCodingBrowserNodeInside(call, handler) {
		return false
	}
	for current := call; current != nil && current.Id() != handler.Id(); {
		parent := current.Parent()
		if parent == nil || !directCodingBrowserNodeInside(parent, handler) {
			break
		}
		switch parent.Kind() {
		case "if_statement":
			condition, constant := directCodingBrowserConstantBoolean(
				parent.ChildByFieldName("condition"), source,
			)
			if constant && directCodingBrowserNodeInDeadConditionalArm(
				current, parent, condition,
			) {
				return false
			}
		case "ternary_expression":
			condition, constant := directCodingBrowserConstantBoolean(
				parent.ChildByFieldName("condition"), source,
			)
			if constant && directCodingBrowserNodeInDeadConditionalArm(
				current, parent, condition,
			) {
				return false
			}
		case "binary_expression":
			if directCodingBrowserNodeInDeadLogicalRight(current, parent, source) {
				return false
			}
		case "while_statement":
			condition, constant := directCodingBrowserConstantBoolean(
				parent.ChildByFieldName("condition"), source,
			)
			body := parent.ChildByFieldName("body")
			if constant && !condition && directCodingBrowserNodeInside(current, body) {
				return false
			}
		case "statement_block":
			if directCodingBrowserStatementFollowsTerminator(current, parent) {
				return false
			}
		}
		current = parent
	}
	return true
}

func directCodingBrowserConstantBoolean(
	node *treesitter.Node,
	source []byte,
) (bool, bool) {
	node = directCodingBrowserUnwrapRuntimeExpression(node)
	if node == nil {
		return false, false
	}
	switch node.Kind() {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func directCodingBrowserNodeInDeadConditionalArm(
	node *treesitter.Node,
	conditional *treesitter.Node,
	condition bool,
) bool {
	if node == nil || conditional == nil {
		return false
	}
	consequence := conditional.ChildByFieldName("consequence")
	alternative := conditional.ChildByFieldName("alternative")
	return !condition && directCodingBrowserNodeInside(node, consequence) ||
		condition && directCodingBrowserNodeInside(node, alternative)
}

func directCodingBrowserNodeInDeadLogicalRight(
	node *treesitter.Node,
	binary *treesitter.Node,
	source []byte,
) bool {
	left := binary.ChildByFieldName("left")
	right := binary.ChildByFieldName("right")
	if left == nil || right == nil || !directCodingBrowserNodeInside(node, right) ||
		left.EndByte() > right.StartByte() || right.StartByte() > uint(len(source)) {
		return false
	}
	value, constant := directCodingBrowserConstantBoolean(left, source)
	if !constant {
		return false
	}
	operator := strings.TrimSpace(string(source[left.EndByte():right.StartByte()]))
	return !value && operator == "&&" || value && operator == "||"
}

func directCodingBrowserStatementFollowsTerminator(
	node *treesitter.Node,
	block *treesitter.Node,
) bool {
	if node == nil || block == nil {
		return false
	}
	for index := uint(0); index < block.NamedChildCount(); index++ {
		statement := block.NamedChild(index)
		if statement == nil {
			continue
		}
		if directCodingBrowserNodeInside(node, statement) {
			return false
		}
		if statement.EndByte() <= node.StartByte() &&
			(statement.Kind() == "return_statement" || statement.Kind() == "throw_statement") {
			return true
		}
	}
	return false
}
