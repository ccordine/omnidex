package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

func stationGapSemanticUncertainty(
	kind assemblyline.WorkKind,
) (assemblyline.SemanticUncertaintyContract, string, error) {
	contract, err := assemblyline.SemanticUncertaintyContractForWorkKind(kind)
	if err != nil {
		return assemblyline.SemanticUncertaintyContract{}, "", err
	}
	digest, err := contract.Digest()
	if err != nil {
		return assemblyline.SemanticUncertaintyContract{}, "", err
	}
	return contract, digest, nil
}

// ValidateStationGapSemanticUncertainty verifies the exact code-owned
// uncertainty that justified this opening. It is durable operational evidence,
// never model context.
func ValidateStationGapSemanticUncertainty(opening StationGapOpening) error {
	expected, err := assemblyline.SemanticUncertaintyContractForPortableRenderer(
		opening.RendererVersion, assemblyline.WorkKind(opening.WorkKind),
	)
	if err != nil {
		return fmt.Errorf("resolve station gap semantic uncertainty: %w", err)
	}
	digest, err := expected.Digest()
	if err != nil {
		return fmt.Errorf("digest station gap semantic uncertainty: %w", err)
	}
	if opening.SemanticUncertaintyContract != expected {
		return fmt.Errorf(
			"station gap semantic uncertainty differs from the exact contract for work kind %q",
			opening.WorkKind,
		)
	}
	if opening.SemanticUncertaintyContractSHA256 != digest {
		return fmt.Errorf("station gap semantic uncertainty digest differs from its exact contract")
	}
	return nil
}

func canonicalStationGapSemanticUncertainty(opening StationGapOpening) ([]byte, error) {
	if err := ValidateStationGapSemanticUncertainty(opening); err != nil {
		return nil, err
	}
	return exactjson.Canonical(opening.SemanticUncertaintyContract)
}

func decodeStationGapSemanticUncertainty(
	raw []byte,
	digest string,
	renderer string,
	workKind string,
) (assemblyline.SemanticUncertaintyContract, error) {
	var contract assemblyline.SemanticUncertaintyContract
	if err := exactjson.ValidateObject(raw, contract, "station gap semantic uncertainty"); err != nil {
		return contract, err
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		return contract, fmt.Errorf("decode station gap semantic uncertainty: %w", err)
	}
	opening := StationGapOpening{
		RendererVersion: renderer, WorkKind: workKind,
		SemanticUncertaintyContract:       contract,
		SemanticUncertaintyContractSHA256: digest,
	}
	if err := ValidateStationGapSemanticUncertainty(opening); err != nil {
		return contract, err
	}
	return contract, nil
}
