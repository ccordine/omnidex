package assemblyline

import (
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// TypeScriptRegularExpressionLiterals returns only parser-proven regular
// expression literals from one validated TypeScript source unit. Callers use
// these exact source fragments as provenance, never as inferred semantics.
func TypeScriptRegularExpressionLiterals(source string, tsx bool) ([]string, error) {
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return nil, err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return nil, fmt.Errorf("extract TypeScript regular expressions from invalid syntax")
	}
	seen := make(map[string]struct{})
	literals := []string{}
	collectTypeScriptRegularExpressionLiterals(root, []byte(source), seen, &literals)
	return literals, nil
}

func collectTypeScriptRegularExpressionLiterals(
	node *treesitter.Node,
	source []byte,
	seen map[string]struct{},
	literals *[]string,
) {
	if node == nil {
		return
	}
	if node.Kind() == "regex" {
		literal := node.Utf8Text(source)
		if _, exists := seen[literal]; !exists {
			seen[literal] = struct{}{}
			*literals = append(*literals, literal)
		}
		return
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		collectTypeScriptRegularExpressionLiterals(
			node.NamedChild(index), source, seen, literals,
		)
	}
}
