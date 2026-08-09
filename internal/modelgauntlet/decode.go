package modelgauntlet

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func decodeLens(raw string) (deliberationLens, error) {
	var decision deliberationLensDecision
	if err := decodeExactJSON(raw, &decision); err != nil {
		return "", fmt.Errorf("invalid lens selection: %w", err)
	}
	if decision.Schema != deliberationLensSchemaV1 {
		return "", fmt.Errorf("lens selection schema must be %q", deliberationLensSchemaV1)
	}
	if _, err := lensInstruction(decision.Lens); err != nil {
		return "", err
	}
	return decision.Lens, nil
}

func decodeRelation(raw string, input assemblyline.CapabilityRelationInput) (assemblyline.CapabilityRelation, error) {
	var decision assemblyline.CapabilityRelationDecision
	if err := decodeExactJSON(raw, &decision); err != nil {
		return "", fmt.Errorf("invalid capability relation: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return "", err
	}
	return decision.Relation, nil
}

func decodeExactJSON(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("response is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("response contains trailing JSON")
		}
		return fmt.Errorf("response contains trailing data: %w", err)
	}
	return nil
}
