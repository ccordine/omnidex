package assemblyline

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/gofragment"
	"golang.org/x/tools/go/ast/astutil"
)

// Derive imports before composing source so every retained source span includes
// the exact package/import prefix. Source blocks retain only their declarations.
func goDocumentPreambleWithImports(document SourceDocument, composition SourceComposition) (string, error) {
	paths := map[string]bool{}
	for _, block := range document.Blocks {
		if !block.Generated() {
			continue
		}
		source, exists := composition.Generated[block.ID]
		if !exists || strings.TrimSpace(source) == "" {
			return "", fmt.Errorf("generated block %s has no source", block.ID)
		}
		permitted, err := goBlockPermittedSymbols(block, composition.Interfaces)
		if err != nil {
			return "", err
		}
		imports, err := gofragment.StandardLibraryImports(source, permitted)
		if err != nil {
			return "", fmt.Errorf("resolve generated block %s imports: %w", block.ID, err)
		}
		for _, imported := range imports {
			paths[imported] = true
		}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", composeSourceDocumentPreamble(document)+"\n", parser.ParseComments)
	if err != nil {
		return "", err
	}
	ordered := make([]string, 0, len(paths))
	for imported := range paths {
		ordered = append(ordered, imported)
	}
	sort.Strings(ordered)
	for _, imported := range ordered {
		astutil.AddImport(fset, file, imported)
	}
	var rendered bytes.Buffer
	if err := format.Node(&rendered, fset, file); err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered.String()), nil
}
