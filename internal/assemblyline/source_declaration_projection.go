package assemblyline

import (
	"fmt"
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
