package assemblyline

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/gofragment"
)

// ComposeGoDocument constructs one complete Go source file from code-owned
// document structure and independently validated model-owned function blocks.
func ComposeGoDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	if composition.Generated == nil || composition.Interfaces == nil {
		return ComposedSourceDocument{}, fmt.Errorf("Go composition requires generated source and interface maps")
	}
	if err := validateGoSourceDocument(document); err != nil {
		return ComposedSourceDocument{}, err
	}

	var source strings.Builder
	preamble, err := formatGoPreamble(composeSourceDocumentPreamble(document))
	if err != nil {
		return ComposedSourceDocument{}, fmt.Errorf("format Go document %s preamble: %w", document.ID, err)
	}
	source.WriteString(preamble)
	source.WriteString("\n\n")
	line := strings.Count(preamble, "\n") + 3
	spans := make(map[string]SourceSpan, len(document.Blocks))
	for index, block := range document.Blocks {
		declaration := strings.TrimSpace(block.Static)
		if block.Generated() {
			candidate, exists := composition.Generated[block.ID]
			if !exists || strings.TrimSpace(candidate) == "" {
				return ComposedSourceDocument{}, fmt.Errorf("generated block %s has no source", block.ID)
			}
			permitted, err := goBlockPermittedSymbols(block, composition.Interfaces)
			if err != nil {
				return ComposedSourceDocument{}, err
			}
			declaration, err = gofragment.ParseNewFunction(block.Signature, permitted, candidate)
			if err != nil {
				return ComposedSourceDocument{}, fmt.Errorf("generated block %s: %w", block.ID, err)
			}
		}

		declaration, err = formatGoDeclaration(declaration)
		if err != nil {
			return ComposedSourceDocument{}, fmt.Errorf(
				"format Go document %s block %s: %w", document.ID, block.ID, err,
			)
		}
		start := line
		source.WriteString(declaration)
		source.WriteString("\n")
		end := line + strings.Count(declaration, "\n")
		spans[block.ID] = SourceSpan{StartLine: start, EndLine: end}
		line = end + 1
		if index < len(document.Blocks)-1 {
			source.WriteString("\n")
			line++
		}
	}
	assembled := source.String()
	if _, err := parser.ParseFile(token.NewFileSet(), document.Path, assembled, parser.AllErrors); err != nil {
		return ComposedSourceDocument{}, fmt.Errorf("parse assembled Go document %s: %w", document.ID, err)
	}
	return ComposedSourceDocument{
		ID: document.ID, Path: document.Path, Source: assembled, Spans: spans,
	}, nil
}

func formatGoPreamble(source string) (string, error) {
	formatted, err := format.Source([]byte(strings.TrimSpace(source) + "\n"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(formatted)), nil
}

func formatGoDeclaration(source string) (string, error) {
	const prefix = "package fragment\n\n"
	formatted, err := format.Source([]byte(prefix + strings.TrimSpace(source) + "\n"))
	if err != nil {
		return "", err
	}
	value := string(formatted)
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("formatted declaration lost its code-owned package prefix")
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix)), nil
}

// ValidateGoSourceBlueprint proves that every document has a Go path,
// package/import preamble, and only source authorities the Go composer can
// enforce before any generated function is requested.
func ValidateGoSourceBlueprint(blueprint SourceBlueprint) error {
	if err := blueprint.Validate(); err != nil {
		return err
	}
	for _, document := range blueprint.Documents {
		if err := validateGoSourceDocument(document); err != nil {
			return err
		}
	}
	return nil
}

func validateGoSourceDocument(document SourceDocument) error {
	if strings.TrimSpace(document.Postamble) != "" {
		return fmt.Errorf("document %s uses unsupported Go postamble authority", document.ID)
	}
	if path.Ext(document.Path) != ".go" {
		return fmt.Errorf("document %s path %q must be Go source", document.ID, document.Path)
	}
	if err := validateGoDocumentPreamble(document); err != nil {
		return err
	}
	for index, block := range document.Blocks {
		if err := validateSourceBlock(block); err != nil {
			return fmt.Errorf("document %s block %d: %w", document.ID, index, err)
		}
		if err := validateGoSourceBlock(block); err != nil {
			return fmt.Errorf("document %s block %s: %w", document.ID, block.ID, err)
		}
	}
	return nil
}

func validateGoDocumentPreamble(document SourceDocument) error {
	base := strings.TrimSpace(document.Preamble)
	if base == "" {
		return fmt.Errorf("Go document %s requires a package preamble", document.ID)
	}
	if err := validateGoPackageAndImports(document.ID, document.Path, base); err != nil {
		return err
	}
	for index, scoped := range document.ScopedPreambles {
		source := "package fragment\n" + strings.TrimSpace(scoped.Source) + "\n"
		parsed, err := parser.ParseFile(token.NewFileSet(), document.Path, source, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("parse Go document %s scoped preamble %d: %w", document.ID, index, err)
		}
		if err := requireOnlyGoImports(document.ID, parsed.Decls); err != nil {
			return fmt.Errorf("scoped preamble %d: %w", index, err)
		}
	}
	return validateGoPackageAndImports(
		document.ID, document.Path, composeSourceDocumentPreamble(document),
	)
}

func validateGoPackageAndImports(documentID, documentPath, source string) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), documentPath, source+"\n", parser.AllErrors)
	if err != nil {
		return fmt.Errorf("parse Go document %s preamble: %w", documentID, err)
	}
	if parsed.Name == nil || parsed.Name.Name == "" {
		return fmt.Errorf("Go document %s preamble has no package name", documentID)
	}
	return requireOnlyGoImports(documentID, parsed.Decls)
}

func requireOnlyGoImports(documentID string, declarations []ast.Decl) error {
	for _, declaration := range declarations {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.IMPORT {
			return fmt.Errorf("Go document %s preamble may contain only package and import declarations", documentID)
		}
	}
	return nil
}

func validateGoSourceBlock(block SourceBlock) error {
	if block.Export {
		return fmt.Errorf("export authority is encoded by the exact Go signature")
	}
	if !emptyGoSourcePolicy(block.Policy) {
		return fmt.Errorf("Go source policy is unsupported by the registered parser")
	}
	if block.Generated() {
		if _, err := gofragment.CompileNewFunctionSignature(block.Signature); err != nil {
			return err
		}
		return nil
	}
	parsed, err := parser.ParseFile(
		token.NewFileSet(), "", "package fragment\n\n"+strings.TrimSpace(block.Static)+"\n", parser.AllErrors,
	)
	if err != nil {
		return fmt.Errorf("parse static Go source: %w", err)
	}
	if len(parsed.Decls) == 0 {
		return fmt.Errorf("static Go source contains no declaration")
	}
	for _, declaration := range parsed.Decls {
		if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			return fmt.Errorf("static Go block imports must belong to the document preamble")
		}
	}
	return nil
}

func goBlockPermittedSymbols(block SourceBlock, interfaces map[string]string) ([]string, error) {
	permitted := append([]string(nil), block.Globals...)
	for _, capability := range block.Capabilities {
		declaration := strings.TrimSpace(interfaces[capability])
		if declaration == "" {
			return nil, fmt.Errorf("generated block %s capability %s has no accepted API", block.ID, capability)
		}
		permitted = append(permitted, declaration)
	}
	return permitted, nil
}

func emptyGoSourcePolicy(policy SourceFunctionPolicy) bool {
	return len(policy.RequiredCalls) == 0 && len(policy.RestrictedCalls) == 0 &&
		len(policy.TopLevelCalls) == 0 && len(policy.RequiredElementNames) == 0 &&
		len(policy.ForbiddenIdentifiers) == 0
}
