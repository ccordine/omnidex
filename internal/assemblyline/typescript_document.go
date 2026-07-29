package assemblyline

import (
	"fmt"
	"strings"
)

type ComposedTypeScriptDocument struct {
	ID     string
	Path   string
	Source string
	Spans  map[string]SourceSpan
}

func ComposeTypeScriptDocument(document TypeScriptDocument, generated map[string]string) (ComposedTypeScriptDocument, error) {
	// Cross-document dependencies are validated by the complete blueprint. The
	// composer only validates local syntax and block authority.
	for index, block := range document.Blocks {
		if err := validateTypeScriptBlock(block); err != nil {
			return ComposedTypeScriptDocument{}, fmt.Errorf("block %d: %w", index, err)
		}
	}
	var source strings.Builder
	line := 1
	header := strings.TrimSpace(document.Header)
	if header != "" {
		source.WriteString(header)
		source.WriteString("\n\n")
		line += strings.Count(header, "\n") + 2
	}
	spans := make(map[string]SourceSpan, len(document.Blocks))
	for index, block := range document.Blocks {
		declaration := strings.TrimSpace(block.Static)
		if block.Generated() {
			fragment, err := ParseTypeScriptFunction(TypeScriptFunctionContract{
				Signature: block.Signature, TSX: document.TSX(), Policy: block.Policy,
			}, generated[block.ID])
			if err != nil {
				return ComposedTypeScriptDocument{}, fmt.Errorf("generated block %s: %w", block.ID, err)
			}
			declaration = strings.TrimSpace(fragment.Source)
			if block.Export {
				declaration = "export " + declaration
			}
		} else if err := ValidateTypeScriptSource(declaration, document.TSX()); err != nil {
			return ComposedTypeScriptDocument{}, fmt.Errorf("static block %s: %w", block.ID, err)
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
	if err := ValidateTypeScriptSource(assembled, document.TSX()); err != nil {
		return ComposedTypeScriptDocument{}, fmt.Errorf("parse assembled TypeScript document %s: %w", document.ID, err)
	}
	return ComposedTypeScriptDocument{ID: document.ID, Path: document.Path, Source: assembled, Spans: spans}, nil
}
