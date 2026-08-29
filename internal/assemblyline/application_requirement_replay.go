package assemblyline

import "fmt"

// DecodeApplicationRequirementCoverageLeafForPortableRenderer validates one
// replay response against the sole current portable renderer.
func DecodeApplicationRequirementCoverageLeafForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (string, error) {
	if renderer != PortableRendererV1 {
		return "", fmt.Errorf("portable renderer %q is not registered", renderer)
	}
	var input ApplicationRequirementCoverageInput
	if err := decodePortablePayload(payload, &input); err != nil {
		return "", err
	}
	result, err := DecodeApplicationRequirementCoverageLeaf(input, raw)
	if err != nil {
		return "", err
	}
	return result.Relation, nil
}

// DecodeApplicationRequirementLeafForPortableRenderer validates one replay
// response against the sole current portable renderer.
func DecodeApplicationRequirementLeafForPortableRenderer(
	payload []byte,
	renderer string,
	raw string,
) (string, error) {
	if renderer != PortableRendererV1 {
		return "", fmt.Errorf("portable renderer %q is not registered", renderer)
	}
	var input ApplicationRequirementCandidateInput
	if err := decodePortablePayload(payload, &input); err != nil {
		return "", err
	}
	return DecodeApplicationRequirementLeaf(input, raw)
}
