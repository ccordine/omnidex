package hostbridge

import (
	"fmt"
	"strings"
)

func stringField(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func requiredStringField(payload map[string]any, key string) (string, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return "", fmt.Errorf("host bridge response is missing %q", key)
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("host bridge response field %q must be a nonempty string", key)
	}
	return value, nil
}

func boolField(payload map[string]any, key string) bool {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false
	}
	if value, ok := raw.(bool); ok {
		return value
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(raw)), "true")
}

func requiredBoolField(payload map[string]any, key string) (bool, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false, fmt.Errorf("host bridge response is missing %q", key)
	}
	value, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("host bridge response field %q must be a boolean", key)
	}
	return value, nil
}

func requiredIntField(payload map[string]any, key string) (int, error) {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return 0, fmt.Errorf("host bridge response is missing %q", key)
	}
	value, ok := raw.(float64)
	if !ok || value != float64(int(value)) {
		return 0, fmt.Errorf("host bridge response field %q must be an integer", key)
	}
	return int(value), nil
}
