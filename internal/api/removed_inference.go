package api

import (
	"encoding/json"
	"fmt"
	"io"
)

func requireJSONEOF(decoder *json.Decoder, label string) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s contains trailing JSON", label)
		}
		return fmt.Errorf("%s trailing data: %w", label, err)
	}
	return nil
}
