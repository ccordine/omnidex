package assemblyline

// NewExactSourceDeclarationPortableResultProjection binds one parser-validated
// declaration to the complete untrusted response without discarding or
// normalizing any response byte.
func NewExactSourceDeclarationPortableResultProjection(
	raw string,
) (PortableResultProjection, error) {
	projection := PortableResultProjection{
		Kind: PortableResultProjectionSourceDeclaration, Source: raw,
		SourceResponseSHA256: portableProjectionSHA256(raw),
		SourceSHA256:         portableProjectionSHA256(raw),
		StartByte:            0, EndByte: len(raw), RawBytes: len(raw),
	}
	if err := projection.ValidateFor(raw); err != nil {
		return PortableResultProjection{}, err
	}
	return projection, nil
}
