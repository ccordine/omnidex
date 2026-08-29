package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

func decodePortablePayload(payload []byte, target any) error {
	if !utf8.Valid(payload) {
		return fmt.Errorf("decode portable job payload: invalid UTF-8")
	}
	if err := exactjson.ValidateObject(payload, target, "portable job payload"); err != nil {
		return fmt.Errorf("decode portable job payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode portable job payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode portable job payload: trailing JSON value")
		}
		return fmt.Errorf("decode portable job payload: %w", err)
	}
	return nil
}
