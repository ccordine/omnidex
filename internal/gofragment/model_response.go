package gofragment

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// FunctionModelResponseProjection binds one exact declaration to the complete
// untrusted model response. A valid projection never discards or normalizes
// response bytes.
type FunctionModelResponseProjection struct {
	Source    string
	StartByte int
	EndByte   int
}

func ProjectFunctionModelResponseProjection(
	raw string,
) (FunctionModelResponseProjection, error) {
	var zero FunctionModelResponseProjection
	if raw == "" {
		return zero, fmt.Errorf("Go model response is empty")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return zero, fmt.Errorf("Go model response must be valid UTF-8 without NUL bytes")
	}
	if raw != strings.TrimSpace(raw) {
		return zero, fmt.Errorf("Go model response must contain only one exact raw declaration")
	}
	if _, err := parseOneFunction(raw, false); err != nil {
		return zero, err
	}
	return FunctionModelResponseProjection{
		Source: raw, StartByte: 0, EndByte: len(raw),
	}, nil
}
