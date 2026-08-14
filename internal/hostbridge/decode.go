package hostbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const maxResponseBodyBytes = 1 << 20

func decodeResponseJSON(raw []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	if err := exactjson.ValidateUniqueObject(trimmed, "host bridge response"); err != nil {
		return nil, fmt.Errorf("invalid host bridge JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		snippet := strings.TrimSpace(string(trimmed))
		if len(snippet) > 160 {
			snippet = snippet[:160] + "…"
		}
		return nil, fmt.Errorf("invalid host bridge JSON (%v): %s", err, snippet)
	}
	if payload == nil {
		return nil, fmt.Errorf("invalid host bridge JSON: object is required")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("invalid host bridge JSON: trailing bytes: %w", err)
	}
	return payload, nil
}

func decodeResponseBody(raw []byte, statusCode int) (map[string]any, error) {
	payload, err := decodeResponseJSON(raw)
	if err != nil {
		if statusCode < 200 || statusCode >= 300 {
			snippet := strings.TrimSpace(string(bytes.TrimSpace(raw)))
			if len(snippet) > 160 {
				snippet = snippet[:160] + "…"
			}
			if snippet == "" {
				snippet = http.StatusText(statusCode)
			}
			return nil, fmt.Errorf("host bridge HTTP %d: %s", statusCode, snippet)
		}
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		message := stringField(payload, "error")
		if message == "" {
			message = fmt.Sprintf("host bridge HTTP %d", statusCode)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return payload, nil
}

func readResponseBody(r io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: r, N: maxResponseBodyBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBodyBytes {
		return nil, fmt.Errorf("host bridge response exceeds the %d-byte limit", maxResponseBodyBytes)
	}
	return raw, nil
}
