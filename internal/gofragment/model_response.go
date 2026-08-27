package gofragment

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// FunctionModelResponseProjection binds the selected declaration to its exact
// byte span in the complete untrusted response. Source is never formatted or
// normalized at this evidence boundary.
type FunctionModelResponseProjection struct {
	Source    string
	StartByte int
	EndByte   int
}

func ProjectFunctionModelResponseProjection(
	raw string,
) (FunctionModelResponseProjection, error) {
	var zero FunctionModelResponseProjection
	if raw == "" || strings.TrimSpace(raw) == "" {
		return zero, fmt.Errorf("Go model response is empty")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return zero, fmt.Errorf("Go model response must be valid UTF-8 without NUL bytes")
	}
	trimmed := strings.TrimSpace(raw)
	trimmedStart := strings.Index(raw, trimmed)
	if trimmedStart < 0 {
		return zero, fmt.Errorf("Go declaration is not an exact response span")
	}
	if !strings.HasPrefix(trimmed, "```") {
		return projectFunctionDeclaration(
			raw, trimmedStart, trimmedStart+len(trimmed),
		)
	}
	firstNewline := strings.IndexByte(trimmed, '\n')
	if firstNewline < 0 {
		return zero, fmt.Errorf("Go model response contains an incomplete code fence")
	}
	opening := strings.ToLower(strings.TrimSpace(trimmed[:firstNewline]))
	if opening != "```" && opening != "```go" && opening != "```golang" {
		return zero, fmt.Errorf("Go model response contains an unsupported code fence")
	}
	bodyAndClose := trimmed[firstNewline+1:]
	lastNewline := strings.LastIndexByte(bodyAndClose, '\n')
	if lastNewline < 0 || strings.TrimSpace(bodyAndClose[lastNewline+1:]) != "```" {
		return zero, fmt.Errorf("Go model response contains an unclosed or trailing code fence")
	}
	rawBody := bodyAndClose[:lastNewline]
	body := strings.TrimSpace(rawBody)
	if body == "" {
		return zero, fmt.Errorf("Go model response must contain exactly one fenced declaration")
	}
	bodyStart := strings.Index(rawBody, body)
	if bodyStart < 0 {
		return zero, fmt.Errorf("Go fenced declaration is not an exact response span")
	}
	startByte := trimmedStart + firstNewline + 1 + bodyStart
	return projectFunctionDeclaration(raw, startByte, startByte+len(body))
}

func projectFunctionDeclaration(
	raw string,
	startByte int,
	endByte int,
) (FunctionModelResponseProjection, error) {
	var zero FunctionModelResponseProjection
	if startByte < 0 || endByte <= startByte || endByte > len(raw) {
		return zero, fmt.Errorf("Go declaration projection produced an invalid source span")
	}
	source := raw[startByte:endByte]
	if _, err := parseOneFunction(source, false); err != nil {
		return zero, err
	}
	return FunctionModelResponseProjection{
		Source: source, StartByte: startByte, EndByte: endByte,
	}, nil
}
