package gofragment

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ProjectFunctionModelResponse selects the sole Go declaration payload from
// one untrusted model response. Markdown framing is transport text, not source;
// mixed prose, multiple fences, and non-Go fences remain explicit failures.
func ProjectFunctionModelResponse(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("Go model response is empty")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("Go model response must be valid UTF-8 without NUL bytes")
	}
	trimmed := strings.TrimSpace(raw)
	if !strings.Contains(trimmed, "```") {
		return trimmed, nil
	}
	firstNewline := strings.IndexByte(trimmed, '\n')
	if firstNewline < 0 {
		return "", fmt.Errorf("Go model response contains an incomplete code fence")
	}
	opening := strings.ToLower(strings.TrimSpace(trimmed[:firstNewline]))
	if opening != "```" && opening != "```go" && opening != "```golang" {
		return "", fmt.Errorf("Go model response contains an unsupported code fence")
	}
	bodyAndClose := trimmed[firstNewline+1:]
	lastNewline := strings.LastIndexByte(bodyAndClose, '\n')
	if lastNewline < 0 || strings.TrimSpace(bodyAndClose[lastNewline+1:]) != "```" {
		return "", fmt.Errorf("Go model response contains an unclosed or trailing code fence")
	}
	body := strings.TrimSpace(bodyAndClose[:lastNewline])
	if body == "" || strings.Contains(body, "```") {
		return "", fmt.Errorf("Go model response must contain exactly one fenced declaration")
	}
	return body, nil
}
