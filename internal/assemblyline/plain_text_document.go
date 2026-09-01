package assemblyline

import (
	"fmt"
	"path"
	"strings"
)

const PlainTextAdapterID = "plain_text"

// ComposePlainTextDocument constructs one document from independently owned
// text nodes. Code applies the LF framing to generated plain-text responses.
func ComposePlainTextDocument(
	document SourceDocument,
	composition SourceComposition,
) (ComposedSourceDocument, error) {
	if composition.Generated == nil || composition.Interfaces == nil {
		return ComposedSourceDocument{}, fmt.Errorf(
			"plain-text composition requires generated source and interface maps",
		)
	}
	if err := validatePlainTextSourceDocument(document); err != nil {
		return ComposedSourceDocument{}, err
	}

	var source strings.Builder
	spans := make(map[string]SourceSpan, len(document.Blocks))
	line := 1
	for _, block := range document.Blocks {
		node := block.Static
		if block.Generated() {
			candidate, exists := composition.Generated[block.ID]
			if !exists {
				return ComposedSourceDocument{}, fmt.Errorf(
					"generated block %s has no text node", block.ID,
				)
			}
			var err error
			node, err = NormalizeTextFragmentResponse(candidate)
			if err != nil {
				return ComposedSourceDocument{}, fmt.Errorf(
					"plain-text block %s response: %w", block.ID, err,
				)
			}
		}
		if err := ValidateTextFragment(node); err != nil {
			return ComposedSourceDocument{}, fmt.Errorf(
				"plain-text block %s: %w", block.ID, err,
			)
		}

		start := line
		source.WriteString(node)
		line += strings.Count(node, "\n")
		spans[block.ID] = SourceSpan{StartLine: start, EndLine: line - 1}
	}
	assembled := source.String()
	if err := ValidateTextFragment(assembled); err != nil {
		return ComposedSourceDocument{}, fmt.Errorf(
			"validate assembled plain-text document %s: %w", document.ID, err,
		)
	}
	return ComposedSourceDocument{
		ID: document.ID, Path: document.Path, Source: assembled, Spans: spans,
	}, nil
}

// ValidatePlainTextSourceBlueprint proves that the blueprint contains only
// plain-text documents and source-node authorities the composer enforces.
func ValidatePlainTextSourceBlueprint(blueprint SourceBlueprint) error {
	if err := blueprint.Validate(); err != nil {
		return err
	}
	for _, document := range blueprint.Documents {
		if err := validatePlainTextSourceDocument(document); err != nil {
			return err
		}
	}
	return nil
}

func validatePlainTextSourceDocument(document SourceDocument) error {
	if document.AdapterID != "" && document.AdapterID != PlainTextAdapterID {
		return fmt.Errorf(
			"plain-text document %s claims adapter %q", document.ID, document.AdapterID,
		)
	}
	if !plainTextSourcePath(document.Path) {
		return fmt.Errorf(
			"document %s path %q must be stable plain text", document.ID, document.Path,
		)
	}
	if document.Preamble != "" || len(document.ScopedPreambles) != 0 || document.Postamble != "" {
		return fmt.Errorf("plain-text document %s cannot contain document framing", document.ID)
	}
	if len(document.Blocks) == 0 {
		return fmt.Errorf("plain-text document %s requires at least one text block", document.ID)
	}
	for index, block := range document.Blocks {
		if err := validateSourceBlock(block); err != nil {
			return fmt.Errorf("document %s block %d: %w", document.ID, index, err)
		}
		if err := validatePlainTextSourceBlock(block); err != nil {
			return fmt.Errorf("document %s block %s: %w", document.ID, block.ID, err)
		}
	}
	return nil
}

func validatePlainTextSourceBlock(block SourceBlock) error {
	if len(block.DependsOn) != 0 || len(block.Capabilities) != 0 || len(block.Globals) != 0 {
		return fmt.Errorf("plain-text nodes cannot declare source dependencies or capabilities")
	}
	if !sourceFunctionPolicyIsEmpty(block.Policy) || block.Export {
		return fmt.Errorf("plain-text nodes cannot declare source policy or export authority")
	}
	if block.Generated() {
		if block.Signature != TextFragmentSignature {
			return fmt.Errorf(
				"generated plain-text node requires signature %q", TextFragmentSignature,
			)
		}
		return nil
	}
	return ValidateTextFragment(block.Static)
}

func plainTextSourcePath(value string) bool {
	base := path.Base(value)
	return strings.EqualFold(path.Ext(base), ".txt") ||
		base == ".gitignore" || base == ".dockerignore"
}
