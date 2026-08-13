package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func strictV3MetadataObject(metadata []byte) (map[string]any, error) {
	if len(metadata) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(metadata)))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode coding job metadata: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("coding job metadata must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode coding job metadata: multiple JSON values")
		}
		return nil, fmt.Errorf("decode coding job metadata: %w", err)
	}
	return payload, nil
}

func strictMetadataBool(metadata map[string]any, key string) (bool, bool, error) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return false, false, nil
	}
	flag, ok := value.(bool)
	if !ok {
		return false, true, fmt.Errorf("job metadata %s must be a boolean", key)
	}
	return flag, true, nil
}
