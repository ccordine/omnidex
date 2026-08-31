package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/modelref"
)

const (
	maxNetworkSettingsBodyBytes int64 = 4 * 1024
	maxOllamaPullBodyBytes      int64 = 4 * 1024
)

type networkSettingsRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type ollamaPullRequest struct {
	Model string `json:"model"`
}

func decodeNetworkSettingsRequest(w http.ResponseWriter, r *http.Request) (networkSettingsRequest, error) {
	raw, err := readExactSettingsBody(w, r, maxNetworkSettingsBodyBytes, "network settings request")
	if err != nil {
		return networkSettingsRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, networkSettingsRequest{}, "network settings request"); err != nil {
		return networkSettingsRequest{}, err
	}
	var request networkSettingsRequest
	if err := decodeExactSettingsJSON(raw, "network settings request", &request); err != nil {
		return networkSettingsRequest{}, err
	}
	if request.Host == "" || request.Host != strings.TrimSpace(request.Host) ||
		strings.ContainsAny(request.Host, "\x00/\\?#@\r\n\t ") {
		return networkSettingsRequest{}, fmt.Errorf("network settings host must be one canonical host name or IP address")
	}
	if request.Port < 1 || request.Port > 65535 {
		return networkSettingsRequest{}, fmt.Errorf("network settings port must be between 1 and 65535")
	}
	return request, nil
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
