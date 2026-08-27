package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// NewSourceDeclarationPortableResultProjection binds one parser-selected
// declaration to its exact byte span in the complete untrusted response.
// Source is never normalized or reconstructed at this evidence boundary.
func NewSourceDeclarationPortableResultProjection(
	raw string,
	source string,
	startByte int,
	endByte int,
) (PortableResultProjection, error) {
	projection := PortableResultProjection{
		Kind: PortableResultProjectionSourceDeclaration, Source: source,
		SourceResponseSHA256: portableProjectionSHA256(raw),
		SourceSHA256:         portableProjectionSHA256(source),
		StartByte:            startByte, EndByte: endByte,
		RawBytes: len(raw), DiscardedBytes: len(raw) - len(source),
	}
	if projection.EndByte-projection.StartByte != len(projection.Source) ||
		projection.RawBytes-projection.DiscardedBytes != len(projection.Source) {
		return PortableResultProjection{}, fmt.Errorf(
			"source declaration projection metadata is internally inconsistent",
		)
	}
	if err := projection.ValidateFor(raw); err != nil {
		return PortableResultProjection{}, err
	}
	return projection, nil
}

// ProjectTrimmedSourceDeclarationResponse selects the exact non-whitespace
// response span. A registered language validator must still prove that the
// selected bytes are exactly one permitted declaration before acceptance.
func ProjectTrimmedSourceDeclarationResponse(
	raw string,
) (PortableResultProjection, error) {
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return PortableResultProjection{}, fmt.Errorf(
			"source declaration response must be valid UTF-8 without NUL bytes",
		)
	}
	source := strings.TrimSpace(raw)
	if source == "" {
		return PortableResultProjection{}, fmt.Errorf("source declaration response is empty")
	}
	startByte := strings.Index(raw, source)
	if startByte < 0 {
		return PortableResultProjection{}, fmt.Errorf(
			"source declaration is not an exact response span",
		)
	}
	return NewSourceDeclarationPortableResultProjection(
		raw, source, startByte, startByte+len(source),
	)
}
