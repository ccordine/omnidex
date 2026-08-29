package worker

import (
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func rustParseAcceptanceExpression(
	expression string,
) ([]byte, *treesitter.Node, func(), bool) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, nil, nil, false
	}
	const prefix = "fn __omnidex_predicate() { let __omnidex_value = "
	source := []byte(prefix + expression + "; }")
	root, closeTree, err := parseRustAuthorityTree(source)
	if err != nil {
		return nil, nil, nil, false
	}
	var value *treesitter.Node
	declarations := 0
	walkRustTree(root, func(node *treesitter.Node) {
		if node.Kind() != "let_declaration" {
			return
		}
		pattern := node.ChildByFieldName("pattern")
		if pattern == nil || rustNodeText(source, pattern) != "__omnidex_value" {
			return
		}
		declarations++
		value = node.ChildByFieldName("value")
	})
	if declarations != 1 || value == nil ||
		strings.TrimSpace(rustNodeText(source, value)) != expression {
		closeTree()
		return nil, nil, nil, false
	}
	return source, value, closeTree, true
}

func rustAcceptanceCanonical(source []byte, node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "parenthesized_expression" && node.NamedChildCount() == 1 {
		return rustAcceptanceCanonical(source, node.NamedChild(0))
	}
	if node.ChildCount() == 0 {
		return node.Kind() + ":" + strings.TrimSpace(rustNodeText(source, node))
	}
	var canonical strings.Builder
	canonical.WriteString(node.Kind())
	canonical.WriteByte('(')
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child == nil || child.Kind() == "line_comment" || child.Kind() == "block_comment" {
			continue
		}
		canonical.WriteString(rustAcceptanceCanonical(source, child))
		canonical.WriteByte(';')
	}
	canonical.WriteByte(')')
	return canonical.String()
}

func rustAcceptanceMacroPayload(invocation string) string {
	start := strings.IndexByte(invocation, '(')
	end := strings.LastIndexByte(invocation, ')')
	if start < 0 || end <= start {
		return ""
	}
	return invocation[start+1 : end]
}

func rustAcceptanceSplitArguments(payload string) ([]string, bool) {
	arguments := make([]string, 0, 2)
	start := 0
	depth := 0
	quote := rune(0)
	escaped := false
	for index, character := range payload {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				arguments = append(arguments, strings.TrimSpace(payload[start:index]))
				start = index + 1
			}
		}
	}
	if quote != 0 || depth != 0 {
		return nil, false
	}
	arguments = append(arguments, strings.TrimSpace(payload[start:]))
	return arguments, true
}
