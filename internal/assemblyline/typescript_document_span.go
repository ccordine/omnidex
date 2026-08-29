package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// BlockSource returns the exact source owned by one composed block. The
// composer records inclusive one-based line spans; callers must not broaden a
// block-local parser question to the surrounding document.
func (document ComposedSourceDocument) BlockSource(blockID string) (string, error) {
	if blockID == "" || blockID != strings.TrimSpace(blockID) {
		return "", fmt.Errorf("composed TypeScript block ID is required and must be trimmed")
	}
	span, exists := document.Spans[blockID]
	if !exists {
		return "", fmt.Errorf("composed TypeScript document %s has no block %s", document.ID, blockID)
	}
	if err := validateComposedSourceSpans(document.Source, document.Spans); err != nil {
		return "", fmt.Errorf("composed TypeScript document %s: %w", document.ID, err)
	}
	source, err := sliceComposedSourceSpan(document.Source, span)
	if err != nil {
		return "", fmt.Errorf("composed TypeScript block %s: %w", blockID, err)
	}
	return source, nil
}

func validateComposedSourceSpans(source string, spans map[string]SourceSpan) error {
	for blockID, span := range spans {
		if blockID == "" || blockID != strings.TrimSpace(blockID) {
			return fmt.Errorf("source span has an invalid block ID")
		}
		if span.StartLine < 1 || span.EndLine < span.StartLine {
			return fmt.Errorf("block %s has invalid source span %+v", blockID, span)
		}
		if _, err := sliceComposedSourceSpan(source, span); err != nil {
			return fmt.Errorf("block %s has invalid source span: %w", blockID, err)
		}
		for otherID, other := range spans {
			if blockID >= otherID {
				continue
			}
			if span.StartLine <= other.EndLine && other.StartLine <= span.EndLine {
				return fmt.Errorf("blocks %s and %s have overlapping source spans", blockID, otherID)
			}
		}
	}
	return nil
}

func sliceComposedSourceSpan(source string, span SourceSpan) (string, error) {
	if source == "" {
		return "", fmt.Errorf("source is empty")
	}
	if !utf8.ValidString(source) {
		return "", fmt.Errorf("source is not valid UTF-8")
	}
	start := 0
	for line := 1; line < span.StartLine; line++ {
		relative := strings.IndexByte(source[start:], '\n')
		if relative < 0 {
			return "", fmt.Errorf("start line %d exceeds source", span.StartLine)
		}
		start += relative + 1
		if start == len(source) {
			return "", fmt.Errorf("start line %d names the synthetic line after a terminal newline", span.StartLine)
		}
	}
	endSearch := start
	end := -1
	for line := span.StartLine; line <= span.EndLine; line++ {
		relative := strings.IndexByte(source[endSearch:], '\n')
		if line == span.EndLine {
			if relative < 0 {
				end = len(source)
			} else {
				end = endSearch + relative
				if end > start && source[end-1] == '\r' {
					end--
				}
			}
			break
		}
		if relative < 0 {
			return "", fmt.Errorf("end line %d exceeds source", span.EndLine)
		}
		endSearch += relative + 1
		if endSearch == len(source) {
			return "", fmt.Errorf("end line %d names the synthetic line after a terminal newline", span.EndLine)
		}
	}
	if end <= start {
		return "", fmt.Errorf("source span resolves to an empty block")
	}
	sliced := source[start:end]
	if !utf8.ValidString(sliced) {
		return "", fmt.Errorf("source span is not valid UTF-8")
	}
	return sliced, nil
}
