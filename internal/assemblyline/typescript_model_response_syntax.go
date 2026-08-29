package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func parseTypeScriptResponseTree(
	source string,
	tsx bool,
) (*treesitter.Parser, *treesitter.Tree, error) {
	parser := treesitter.NewParser()
	languagePointer := typescript.LanguageTypescript()
	if tsx {
		languagePointer = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer)); err != nil {
		parser.Close()
		return nil, nil, fmt.Errorf("configure TypeScript response projector: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		parser.Close()
		return nil, nil, fmt.Errorf("TypeScript response projector returned no syntax tree")
	}
	return parser, tree, nil
}

func typeScriptProjectionSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
