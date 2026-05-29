package hostbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decodeResponseJSON(raw []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty response body")
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
	return payload, nil
}

func decodeResponseBody(raw []byte, statusCode int) (map[string]any, error) {
	payload, err := decodeResponseJSON(raw)
	if err != nil {
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
	return io.ReadAll(io.LimitReader(r, 1<<20))
}
