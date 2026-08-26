package assemblyline

import (
	"fmt"
	"path"
	"strings"
)

type ComposedSourceDocument struct {
	ID     string
	Path   string
	Source string
	Spans  map[string]SourceSpan
}

func ComposeTypeScriptDocument(document SourceDocument, composition SourceComposition) (ComposedSourceDocument, error) {
	if composition.Generated == nil || composition.Interfaces == nil {
		return ComposedSourceDocument{}, fmt.Errorf("TypeScript composition requires generated source and interface maps")
	}
	if err := validateTypeScriptSourceDocumentPath(document); err != nil {
		return ComposedSourceDocument{}, err
	}
	// Cross-document dependencies are validated by the complete blueprint. The
	// composer only validates local syntax and block authority.
	for index, block := range document.Blocks {
		if err := validateSourceBlock(block); err != nil {
			return ComposedSourceDocument{}, fmt.Errorf("block %d: %w", index, err)
		}
	}
	var source strings.Builder
	line := 1
	preamble := composeSourceDocumentPreamble(document)
	if preamble != "" {
		source.WriteString(preamble)
		source.WriteString("\n\n")
		line += strings.Count(preamble, "\n") + 2
	}
	spans := make(map[string]SourceSpan, len(document.Blocks))
	for index, block := range document.Blocks {
		declaration := strings.TrimSpace(block.Static)
		if block.Generated() {
			fragment, err := ParseTypeScriptFunction(TypeScriptFunctionContract{
				Signature: block.Signature, TSX: typeScriptSourceDocumentUsesTSX(document), Policy: block.Policy,
			}, composition.Generated[block.ID])
			if err != nil {
				return ComposedSourceDocument{}, fmt.Errorf("generated block %s: %w", block.ID, err)
			}
			declaration = strings.TrimSpace(fragment.Source)
			if block.Export {
				declaration = "export " + declaration
			}
		} else if err := ValidateTypeScriptSource(declaration, typeScriptSourceDocumentUsesTSX(document)); err != nil {
			return ComposedSourceDocument{}, fmt.Errorf("static block %s: %w", block.ID, err)
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
	if err := ValidateTypeScriptSource(assembled, typeScriptSourceDocumentUsesTSX(document)); err != nil {
		return ComposedSourceDocument{}, fmt.Errorf("parse assembled TypeScript document %s: %w", document.ID, err)
	}
	return ComposedSourceDocument{ID: document.ID, Path: document.Path, Source: assembled, Spans: spans}, nil
}

func composeSourceDocumentPreamble(document SourceDocument) string {
	parts := make([]string, 0, len(document.ScopedPreambles)+1)
	if value := strings.TrimSpace(document.Preamble); value != "" {
		parts = append(parts, value)
	}
	for _, scoped := range document.ScopedPreambles {
		parts = append(parts, strings.TrimSpace(scoped.Source))
	}
	return strings.Join(parts, "\n")
}

func ValidateTypeScriptSourceBlueprint(blueprint SourceBlueprint) error {
	if err := blueprint.Validate(); err != nil {
		return err
	}
	for _, document := range blueprint.Documents {
		if err := validateTypeScriptSourceDocumentPath(document); err != nil {
			return err
		}
	}
	return nil
}

func validateTypeScriptSourceDocumentPath(document SourceDocument) error {
	if strings.TrimSpace(document.Postamble) != "" {
		return fmt.Errorf("document %s uses unsupported TypeScript postamble authority", document.ID)
	}
	extension := strings.ToLower(path.Ext(document.Path))
	if extension != ".ts" && extension != ".tsx" {
		return fmt.Errorf("document %s path %q must be TypeScript source", document.ID, document.Path)
	}
	return nil
}

func typeScriptSourceDocumentUsesTSX(document SourceDocument) bool {
	return strings.EqualFold(path.Ext(document.Path), ".tsx")
}
