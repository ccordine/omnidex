package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// decodeOrdinarySemanticText accepts one bounded model-authored text value
// without interpreting its presentation. Markdown, source snippets, quoted
// prose, and JSON-shaped text can all be the semantic response itself; typed
// framework structure is supplied by the calling code after this boundary.
func decodeOrdinarySemanticText(label string, raw string, maximum int) (string, error) {
	if maximum < 1 {
		return "", fmt.Errorf("%s has no positive byte bound", label)
	}
	if len(raw) > maxPortableCandidateBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", label, maxPortableCandidateBytes)
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("%s must be valid UTF-8 without NUL bytes", label)
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s is empty", label)
	}
	if len(raw) > maximum {
		return "", fmt.Errorf("%s exceeds %d bytes", label, maximum)
	}
	return raw, nil
}

// decodeRawSemanticLeaf is the strict acceptance boundary for canonical IDs,
// numeric leaves, sentinel-bearing inventories, and individual inventory
// lines. Genuinely free-form semantic text uses decodeOrdinarySemanticText so
// presentation-shaped content is not mistaken for a response packet.
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
