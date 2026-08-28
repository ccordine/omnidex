package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TypeScriptFunctionProjection binds one exact source declaration to the
// complete untrusted model response. A valid projection never discards or
// normalizes response bytes.
type TypeScriptFunctionProjection struct {
	Source         string
	RawSHA256      string
	SourceSHA256   string
	StartByte      int
	EndByte        int
	RawBytes       int
	SourceBytes    int
	DiscardedBytes int
}

// ProjectTypeScriptFunctionModelResponse requires the complete final response
// to be exactly one parseable raw TypeScript function declaration with the
// required name. Signature, policy, compiler, and acceptance checks remain
// downstream and consume these exact bytes.
func ProjectTypeScriptFunctionModelResponse(
	contract TypeScriptFunctionContract,
	raw string,
) (TypeScriptFunctionProjection, error) {
	var zero TypeScriptFunctionProjection
	if raw == "" {
		return zero, fmt.Errorf("TypeScript model response is empty")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return zero, fmt.Errorf("TypeScript model response must be valid UTF-8 without NUL bytes")
	}
	signature := strings.TrimSpace(contract.Signature)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return zero, fmt.Errorf("TypeScript function contract requires one single-line signature")
	}
	expected, closeExpected, err := parseSingleTypeScriptFunction(
		signature+" {}", contract.TSX, false, SourceFunctionPolicy{},
	)
	if err != nil {
		return zero, fmt.Errorf("invalid code-owned TypeScript signature: %w", err)
	}
	defer closeExpected()

	actual, closeActual, err := parseSingleTypeScriptFunction(
		raw, contract.TSX, false, SourceFunctionPolicy{},
	)
	if err != nil {
		return zero, fmt.Errorf(
			"TypeScript model response must be exactly one parseable raw function declaration: %w",
			err,
		)
	}
	defer closeActual()
	if actual.name != expected.name {
		return zero, fmt.Errorf(
			"TypeScript model response contains function %q, required %q",
			actual.name, expected.name,
		)
	}

	digest := typeScriptProjectionSHA256(raw)
	return TypeScriptFunctionProjection{
		Source: raw, RawSHA256: digest, SourceSHA256: digest,
		StartByte: 0, EndByte: len(raw), RawBytes: len(raw),
		SourceBytes: len(raw), DiscardedBytes: 0,
	}, nil
}
