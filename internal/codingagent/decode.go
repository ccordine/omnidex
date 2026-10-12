package codingagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func decodeContract(raw json.RawMessage, target any) error {
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
