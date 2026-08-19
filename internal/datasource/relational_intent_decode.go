package datasource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func DecodeRelationalIntent(snapshot SchemaSnapshot, raw string) (RelationalIntent, error) {
	if len(raw) == 0 || len(raw) > MaxIntentBytes {
		return RelationalIntent{}, fmt.Errorf("relational intent must contain 1..%d bytes", MaxIntentBytes)
	}
	if err := rejectDuplicateJSONKeys([]byte(raw)); err != nil {
		return RelationalIntent{}, fmt.Errorf("decode relational intent: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	var intent RelationalIntent
	if err := decoder.Decode(&intent); err != nil {
		return RelationalIntent{}, fmt.Errorf("decode relational intent: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return RelationalIntent{}, fmt.Errorf("decode relational intent: %w", err)
	}
	if err := intent.Validate(snapshot); err != nil {
		return RelationalIntent{}, err
	}
	return intent, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim(map[json.Delim]json.Delim{'{': '}', '[': ']'}[delimiter]) {
		return fmt.Errorf("unexpected JSON closing delimiter %q", closing)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
