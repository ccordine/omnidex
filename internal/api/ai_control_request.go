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

const maxAIControlRequestBodyBytes int64 = 1024

type aiControlAction string

const (
	aiControlActionPause  aiControlAction = "pause"
	aiControlActionResume aiControlAction = "resume"
)

type aiControlRequest struct {
	Action aiControlAction `json:"action"`
}

func decodeAIControlRequest(w http.ResponseWriter, r *http.Request) (aiControlRequest, error) {
	if r == nil || r.Body == nil {
		return aiControlRequest{}, fmt.Errorf("AI control request body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAIControlRequestBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return aiControlRequest{}, fmt.Errorf("read AI control request: %w", err)
	}
	if !utf8.Valid(raw) {
		return aiControlRequest{}, fmt.Errorf("AI control request must be valid UTF-8")
	}
	if err := exactjson.ValidateObject(raw, aiControlRequest{}, "AI control request"); err != nil {
		return aiControlRequest{}, err
	}
	var request aiControlRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return aiControlRequest{}, fmt.Errorf("decode AI control request: %w", err)
	}
	if err := requireJSONEOF(decoder, "AI control request"); err != nil {
		return aiControlRequest{}, err
	}
	switch request.Action {
	case aiControlActionPause, aiControlActionResume:
		return request, nil
	default:
		return aiControlRequest{}, fmt.Errorf("AI control action must be exactly %q or %q", aiControlActionPause, aiControlActionResume)
	}
}

func aiControlRequestErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
