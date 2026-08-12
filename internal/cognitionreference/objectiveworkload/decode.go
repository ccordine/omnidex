package objectiveworkload

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

func decodePartitionDecision(raw string) (assemblyline.RequirementPartitionDecision, error) {
	var decision assemblyline.RequirementPartitionDecision
	if err := exactjson.ValidateObject(
		[]byte(raw), decision, "requirement partition candidate",
	); err != nil {
		return decision, fmt.Errorf("decode requirement partition candidate: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return decision, fmt.Errorf("decode requirement partition candidate: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return decision, fmt.Errorf("decode requirement partition candidate: trailing JSON value")
		}
		return decision, fmt.Errorf("decode requirement partition candidate trailing data: %w", err)
	}
	return decision, nil
}
