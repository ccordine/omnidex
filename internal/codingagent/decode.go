package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func decodeContract(raw json.RawMessage, target any) error {
	if err := rejectDuplicateContractFields(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode agent contract: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("agent contract contains trailing data")
	}
	return nil
}

// A repeated JSON key would otherwise silently overwrite an earlier source
// disposition or operation. Every object key has exactly one contract value.
func rejectDuplicateContractFields(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return fmt.Errorf("agent contract contains a duplicate or invalid object field")
			}
			seen[key] = true
			if err := rejectDuplicateContractFields(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := rejectDuplicateContractFields(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid contract container")
	}
	_, err = decoder.Token()
	return err
}
