package modelconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Config map[string]string

func FromJSON(raw json.RawMessage) (Config, error) {
	out := Config{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("model config must be a JSON object: %w", err)
	}
	if err := requireEOF(decoder, "model config"); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("model config must be a JSON object, received null")
	}
	for key, rawValue := range payload {
		if _, ok := definitionForKey(key); !ok {
			return nil, fmt.Errorf("model config contains unsupported field %q", key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("model config field %q must be a string: %w", key, err)
		}
		if value == "" || value != strings.TrimSpace(value) {
			return nil, fmt.Errorf("model config field %q must name one canonical model", key)
		}
		out[key] = value
	}
	return out, nil
}

func FromSettingsJSON(raw json.RawMessage) (Config, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Config{}, nil
	}
	var settings map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("project settings must be a JSON object: %w", err)
	}
	if err := requireEOF(decoder, "project settings"); err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("project settings must be a JSON object, received null")
	}
	if nested, ok := settings["model_config"]; ok {
		return FromJSON(nested)
	}
	return Config{}, nil
}

func requireEOF(decoder *json.Decoder, label string) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%s contains trailing JSON", label)
	}
	return fmt.Errorf("%s contains trailing data: %w", label, err)
}

func (c Config) Get(key string) string {
	if c == nil {
		return ""
	}
	return c[key]
}

func (c Config) Clone() Config {
	out := Config{}
	for key, value := range c {
		out[key] = value
	}
	return out
}

func (c Config) ModelNames() []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, definition := range fieldRegistry {
		value := c.Get(definition.Key)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (c Config) ToMap() map[string]string {
	out := make(map[string]string, len(c))
	for key, value := range c {
		out[key] = value
	}
	return out
}

func (c Config) FieldList() []map[string]any {
	items := make([]map[string]any, 0, len(fieldRegistry))
	for _, definition := range fieldRegistry {
		field := definition.Field
		items = append(items, map[string]any{
			"key":         field.Key,
			"label":       field.Label,
			"description": field.Description,
			"env_keys":    append([]string(nil), field.EnvKeys...),
			"options":     append([]string(nil), field.Options...),
			"value":       c.Get(field.Key),
		})
	}
	return items
}
