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
	"github.com/gryph/omnidex/internal/modelref"
)

const (
	maxOllamaPullBodyBytes      int64 = 4 * 1024
)

type ollamaPullRequest struct {
	Model string `json:"model"`
}

func decodeOllamaPullRequest(w http.ResponseWriter, r *http.Request) (ollamaPullRequest, error) {
	raw, err := readExactSettingsBody(w, r, maxOllamaPullBodyBytes, "Ollama pull request")
	if err != nil {
		return ollamaPullRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, ollamaPullRequest{}, "Ollama pull request"); err != nil {
		return ollamaPullRequest{}, err
	}
	var request ollamaPullRequest
	if err := decodeExactSettingsJSON(raw, "Ollama pull request", &request); err != nil {
		return ollamaPullRequest{}, err
	}
	if err := modelref.ValidateOllamaName(request.Model); err != nil {
		return ollamaPullRequest{}, err
	}
	return request, nil
}

func readExactSettingsBody(w http.ResponseWriter, r *http.Request, limit int64, name string) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, fmt.Errorf("%s body is required", name)
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("%s must be valid UTF-8", name)
	}
	return raw, nil
}

func decodeExactSettingsJSON(raw []byte, name string, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return requireJSONEOF(decoder, name)
}

func exactSettingsErrorStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
