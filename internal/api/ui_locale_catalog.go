package api

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

//go:embed web/locales/*.json
var uiLocaleFiles embed.FS

func loadUIMessageCatalog(locale uiLocale) (map[string]string, error) {
	if _, err := uiLocaleOptionFor(locale); err != nil {
		return nil, err
	}
	path := "web/locales/" + string(locale) + ".json"
	raw, err := uiLocaleFiles.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load UI locale %q: %w", locale, err)
	}
	catalog, err := decodeUIMessageCatalog(raw)
	if err != nil {
		return nil, fmt.Errorf("decode UI locale %q: %w", locale, err)
	}
	return catalog, nil
}

func decodeUIMessageCatalog(raw []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("catalog must be a JSON object")
	}
	catalog := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("catalog message key must be a non-empty string")
		}
		if _, exists := catalog[key]; exists {
			return nil, fmt.Errorf("duplicate catalog message key %q", key)
		}
		var message string
		if err := decoder.Decode(&message); err != nil {
			return nil, fmt.Errorf("message %q must be a string: %w", key, err)
		}
		catalog[key] = message
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected JSON token after catalog: %v", token)
	}
	return catalog, nil
}

func uiMessage(catalog map[string]string, locale uiLocale, key string) (string, error) {
	message, ok := catalog[key]
	if !ok {
		return "", fmt.Errorf("UI locale %q has no message %q", locale, key)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("UI locale %q message %q is blank", locale, key)
	}
	return message, nil
}
