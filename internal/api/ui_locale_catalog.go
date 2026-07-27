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

var uiLocaleCatalogs = mustLoadUILocaleCatalogs()

func mustLoadUILocaleCatalogs() map[uiLocale]map[string]string {
	catalogs := make(map[uiLocale]map[string]string, len(supportedUILocaleOptions))
	for _, option := range supportedUILocaleOptions {
		path := "web/locales/" + string(option.Code) + ".json"
		raw, err := uiLocaleFiles.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("load UI locale %q: %v", option.Code, err))
		}
		catalog, err := decodeUIMessageCatalog(raw)
		if err != nil {
			panic(fmt.Sprintf("decode UI locale %q: %v", option.Code, err))
		}
		catalogs[option.Code] = catalog
	}
	if err := validateUILocaleCatalogs(catalogs); err != nil {
		panic("validate UI locale catalogs: " + err.Error())
	}
	return catalogs
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

func validateUILocaleCatalogs(catalogs map[uiLocale]map[string]string) error {
	english, ok := catalogs[uiLocaleEnglish]
	if !ok {
		return fmt.Errorf("English UI catalog is missing")
	}
	if len(english) == 0 {
		return fmt.Errorf("English UI catalog is empty")
	}
	for key, message := range english {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(message) == "" {
			return fmt.Errorf("English UI message %q is blank", key)
		}
	}
	for _, option := range supportedUILocaleOptions {
		catalog, exists := catalogs[option.Code]
		if !exists {
			return fmt.Errorf("UI catalog %q is missing", option.Code)
		}
		for key := range english {
			message, exists := catalog[key]
			if !exists {
				return fmt.Errorf("UI catalog %q is missing message %q", option.Code, key)
			}
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("UI catalog %q message %q is blank", option.Code, key)
			}
		}
		for key := range catalog {
			if _, exists := english[key]; !exists {
				return fmt.Errorf("UI catalog %q has unknown message %q", option.Code, key)
			}
		}
	}
	if len(catalogs) != len(supportedUILocaleOptions) {
		return fmt.Errorf("UI catalog set has %d locales; expected %d", len(catalogs), len(supportedUILocaleOptions))
	}
	return nil
}

func uiMessage(locale uiLocale, key string) (string, error) {
	catalog, ok := uiLocaleCatalogs[locale]
	if !ok {
		return "", fmt.Errorf("unsupported UI locale %q", locale)
	}
	message, ok := catalog[key]
	if !ok {
		return "", fmt.Errorf("UI locale %q has no message %q", locale, key)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("UI locale %q message %q is blank", locale, key)
	}
	return message, nil
}
