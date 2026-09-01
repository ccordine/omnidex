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

const maxGenericCodingEnqueueBodyBytes int64 = 256 * 1024

type enqueueRequest struct {
	Instruction string                 `json:"instruction"`
	Metadata    *genericCodingMetadata `json:"metadata"`
}

type genericCodingMetadata struct {
	ClientCWD string `json:"client_cwd"`
}

func decodeGenericCodingEnqueue(w http.ResponseWriter, r *http.Request) (enqueueRequest, error) {
	if r == nil || r.Body == nil {
		return enqueueRequest{}, fmt.Errorf("coding enqueue request body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxGenericCodingEnqueueBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return enqueueRequest{}, fmt.Errorf("read coding enqueue request: %w", err)
	}
	if !utf8.Valid(raw) {
		return enqueueRequest{}, fmt.Errorf("coding enqueue request must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, enqueueRequest{}, "coding enqueue request"); err != nil {
		return enqueueRequest{}, err
	}
	var request enqueueRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return enqueueRequest{}, fmt.Errorf("decode coding enqueue request: %w", err)
	}
	if err := requireJSONEOF(decoder, "coding enqueue request"); err != nil {
		return enqueueRequest{}, err
	}
	if request.Metadata == nil {
		return enqueueRequest{}, fmt.Errorf("coding enqueue metadata must be one JSON object")
	}
	return request, nil
}

func genericCodingEnqueueStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
