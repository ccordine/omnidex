package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const maxProjectAutoWorkActionBodyBytes int64 = 1024

type projectAutoWorkActionBody struct{}

func decodeProjectAutoWorkActionRequest(w http.ResponseWriter, r *http.Request, name string) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("%s body is required", name)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProjectAutoWorkActionBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%s exceeds the %d-byte transport bound: %w", name, maxProjectAutoWorkActionBodyBytes, err)
		}
		return fmt.Errorf("read %s body: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("%s body must be valid UTF-8", name)
	}
	if err := exactjson.ValidateObject(raw, projectAutoWorkActionBody{}, name+" request"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var body projectAutoWorkActionBody
	if err := decoder.Decode(&body); err != nil {
		return fmt.Errorf("decode %s request: %w", name, err)
	}
	return requireJSONEOF(decoder, name+" request")
}
