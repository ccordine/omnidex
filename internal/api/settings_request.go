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
	"github.com/gryph/omnidex/internal/secrets"
)

const (
	maxModelSettingsBodyBytes   int64 = 64 * 1024
	maxNetworkSettingsBodyBytes int64 = 4 * 1024
	maxAPISecretsBodyBytes      int64 = 64 * 1024
	maxOllamaPullBodyBytes      int64 = 4 * 1024
)

type modelSettingsRequest struct {
	Values map[string]string `json:"values"`
}

type networkSettingsRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type apiSecretsRequest struct {
	Values    map[string]string `json:"values"`
	ClearKeys []string          `json:"clear_keys"`
}

type ollamaPullRequest struct {
	Model string `json:"model"`
}

func decodeModelSettingsRequest(w http.ResponseWriter, r *http.Request) (modelSettingsRequest, error) {
	raw, err := readExactSettingsBody(w, r, maxModelSettingsBodyBytes, "model settings request")
	if err != nil {
		return modelSettingsRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, modelSettingsRequest{}, "model settings request"); err != nil {
		return modelSettingsRequest{}, err
	}
	var shape struct {
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := decodeExactSettingsJSON(raw, "model settings request", &shape); err != nil {
		return modelSettingsRequest{}, err
	}
	if shape.Values == nil {
		return modelSettingsRequest{}, fmt.Errorf("model settings values must be one JSON object")
	}
	request := modelSettingsRequest{Values: make(map[string]string, len(shape.Values))}
	for key, value := range shape.Values {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return modelSettingsRequest{}, fmt.Errorf("model setting %q must be a string, received null", key)
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return modelSettingsRequest{}, fmt.Errorf("model setting %q must be a string: %w", key, err)
		}
		if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
			return modelSettingsRequest{}, fmt.Errorf("model setting %q must be valid UTF-8 without NUL", key)
		}
		request.Values[key] = text
	}
	return request, nil
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

func decodeAPISecretsRequest(w http.ResponseWriter, r *http.Request) (apiSecretsRequest, error) {
	raw, err := readExactSettingsBody(w, r, maxAPISecretsBodyBytes, "API secrets request")
	if err != nil {
		return apiSecretsRequest{}, err
	}
	if err := exactjson.ValidateObject(raw, apiSecretsRequest{}, "API secrets request"); err != nil {
		return apiSecretsRequest{}, err
	}
	var request apiSecretsRequest
	if err := decodeExactSettingsJSON(raw, "API secrets request", &request); err != nil {
		return apiSecretsRequest{}, err
	}
	if request.Values == nil || request.ClearKeys == nil {
		return apiSecretsRequest{}, fmt.Errorf("API secrets values and clear_keys must be JSON collections")
	}
	allowed := make(map[string]struct{}, len(secrets.Fields))
	for _, field := range secrets.Fields {
		allowed[field.Key] = struct{}{}
	}
	for key, value := range request.Values {
		if _, ok := allowed[key]; !ok {
			return apiSecretsRequest{}, fmt.Errorf("API secret field %q is unsupported or retired", key)
		}
		if value == "" || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') {
			return apiSecretsRequest{}, fmt.Errorf("API secret field %q must be a non-empty canonical string without NUL", key)
		}
	}
	seenClear := make(map[string]struct{}, len(request.ClearKeys))
	for _, key := range request.ClearKeys {
		if _, ok := allowed[key]; !ok {
			return apiSecretsRequest{}, fmt.Errorf("API secret clear key %q is unsupported or retired", key)
		}
		if key == "" || key != strings.TrimSpace(key) || strings.ContainsRune(key, '\x00') {
			return apiSecretsRequest{}, fmt.Errorf("API secret clear key must be canonical")
		}
		if _, duplicate := seenClear[key]; duplicate {
			return apiSecretsRequest{}, fmt.Errorf("API secret clear key %q is duplicated", key)
		}
		seenClear[key] = struct{}{}
		if _, conflict := request.Values[key]; conflict {
			return apiSecretsRequest{}, fmt.Errorf("API secret field %q cannot be set and cleared together", key)
		}
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
