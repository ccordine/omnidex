package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserRuntimeRegExpAliases map[uintptr]struct{}

type directCodingBrowserRuntimeRegExpAliasCandidate struct {
	declarationID uintptr
	value         *treesitter.Node
}

func collectDirectCodingBrowserRuntimeRegExpAliases(
	root *treesitter.Node,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) directCodingBrowserRuntimeRegExpAliases {
	aliases := make(directCodingBrowserRuntimeRegExpAliases)
	candidates := make([]directCodingBrowserRuntimeRegExpAliasCandidate, 0)
	var walk func(*treesitter.Node)
	walk = func(node *treesitter.Node) {
		if node == nil {
			return
		}
		switch node.Kind() {
		case "variable_declarator":
			name := node.ChildByFieldName("name")
			if name != nil && name.Kind() == "identifier" {
				candidates = append(candidates, directCodingBrowserRuntimeRegExpAliasCandidate{
					declarationID: name.Id(), value: node.ChildByFieldName("value"),
				})
			}
		case "assignment_expression":
			left := directCodingBrowserUnwrapRuntimeExpression(node.ChildByFieldName("left"))
			if left != nil && left.Kind() == "identifier" {
				binding, resolved := bindings.resolve(
					directCodingBrowserRuntimeNodeText(source, left), left,
				)
				if resolved {
					candidates = append(candidates, directCodingBrowserRuntimeRegExpAliasCandidate{
						declarationID: binding.declarationID,
						value:         node.ChildByFieldName("right"),
					})
				}
			}
		}
		for index := uint(0); index < node.NamedChildCount(); index++ {
			walk(node.NamedChild(index))
		}
	}
	walk(root)
	for changed := true; changed; {
		changed = false
		for _, candidate := range candidates {
			if _, resolved := aliases[candidate.declarationID]; resolved {
				continue
			}
			if aliases.expressionMayBeGlobalRegExp(candidate.value, source, bindings) {
				aliases[candidate.declarationID] = struct{}{}
				changed = true
			}
		}
	}
	return aliases
}

func (aliases directCodingBrowserRuntimeRegExpAliases) rejectsProperty(
	node *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	if !directCodingBrowserRegExpLegacyStaticProperty(name) || node == nil {
		return false
	}
	return aliases.expressionMayBeGlobalRegExp(
		node.ChildByFieldName("object"), source, bindings,
	)
}

func (aliases directCodingBrowserRuntimeRegExpAliases) rejectsPattern(
	node *treesitter.Node,
	name string,
	source []byte,
	bindings directCodingBrowserRuntimeBindings,
) bool {
	if !directCodingBrowserRegExpLegacyStaticProperty(name) {
		return false
	}
	value := directCodingBrowserRuntimePatternSource(node)
	return aliases.expressionMayBeGlobalRegExp(value, source, bindings)
}

func (aliases directCodingBrowserRuntimeRegExpAliases) expressionMayBeGlobalRegExp(
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
		name := directCodingBrowserRuntimeNodeText(source, node)
		if name == "RegExp" && !bindings.binds(name, node) {
			return true
		}
		binding, resolved := bindings.resolve(name, node)
		if !resolved {
			return false
		}
		_, aliased := aliases[binding.declarationID]
		return aliased
	case "sequence_expression":
		if node.NamedChildCount() == 0 {
			return false
		}
		return aliases.expressionMayBeGlobalRegExp(
			node.NamedChild(node.NamedChildCount()-1), source, bindings,
		)
	case "assignment_expression":
		return aliases.expressionMayBeGlobalRegExp(
			node.ChildByFieldName("right"), source, bindings,
		)
	case "ternary_expression":
		return aliases.expressionMayBeGlobalRegExp(
			node.ChildByFieldName("consequence"), source, bindings,
		) || aliases.expressionMayBeGlobalRegExp(
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
		return aliases.expressionMayBeGlobalRegExp(left, source, bindings) ||
			aliases.expressionMayBeGlobalRegExp(right, source, bindings)
	case "await_expression":
		return aliases.expressionMayBeGlobalRegExp(
			node.ChildByFieldName("argument"), source, bindings,
		)
	default:
		return false
	}
}

func directCodingBrowserRuntimePatternSource(node *treesitter.Node) *treesitter.Node {
	for current := node; current != nil; current = current.Parent() {
		switch current.Kind() {
		case "variable_declarator":
			return current.ChildByFieldName("value")
		case "assignment_expression":
			left := current.ChildByFieldName("left")
			if directCodingBrowserNodeInside(node, left) {
				return current.ChildByFieldName("right")
			}
			return nil
		}
		if javaScriptFunctionScopeKind(current.Kind()) {
			return nil
		}
	}
	return nil
}

func directCodingBrowserRegExpLegacyStaticProperty(name string) bool {
	switch name {
	case "input", "$_", "lastMatch", "$&", "lastParen", "$+",
		"leftContext", "$`", "rightContext", "$'":
		return true
	}
	return len(name) == 2 && name[0] == '$' && name[1] >= '1' && name[1] <= '9'
}
