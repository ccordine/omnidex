package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// decodeRawSemanticLeaf is the shared acceptance boundary for model-authored
// semantic text. Structured source nodes use their parser-specific decoders;
// semantic stations return only the requested leaf as plain text.
func decodeRawSemanticLeaf(
	label string,
	raw string,
	maximum int,
	allowMultiline bool,
) (string, error) {
	if maximum < 1 {
		return "", fmt.Errorf("%s raw leaf has no positive byte bound", label)
	}
	if len(raw) > maxPortableCandidateBytes {
		return "", fmt.Errorf("%s raw leaf exceeds %d bytes", label, maxPortableCandidateBytes)
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("%s raw leaf must be valid UTF-8 without NUL bytes", label)
	}
	leaf := raw
	if leaf != strings.TrimSpace(leaf) {
		return "", fmt.Errorf("%s must be an exactly trimmed raw semantic leaf", label)
	}
	if leaf == "" {
		return "", fmt.Errorf("%s raw leaf is empty", label)
	}
	if len(leaf) > maximum {
		return "", fmt.Errorf("%s raw leaf exceeds %d bytes", label, maximum)
	}
	if !allowMultiline && strings.ContainsAny(leaf, "\r\n") {
		return "", fmt.Errorf("%s raw leaf must be exactly one line", label)
	}
	if strings.HasPrefix(leaf, "```") || rawSemanticLeafHasWrapper(leaf) {
		return "", fmt.Errorf("%s must be raw text without JSON, quotes, labels, or Markdown", label)
	}
	return leaf, nil
}

func rawSemanticLeafHasWrapper(leaf string) bool {
	if len(leaf) >= 2 {
		first, last := leaf[0], leaf[len(leaf)-1]
		if first == last && (first == '\'' || first == '`') {
			return true
		}
	}
	var decoded any
	if json.Unmarshal([]byte(leaf), &decoded) != nil {
		return false
	}
	switch decoded.(type) {
	case string, []any, map[string]any, nil:
		return true
	default:
		return false
	}
}
