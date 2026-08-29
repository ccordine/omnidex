package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func decodePortableGapPayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode station gap PortableJob payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode station gap PortableJob payload: trailing JSON")
	}
	return nil
}
