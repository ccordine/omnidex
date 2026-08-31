package secrets

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

const WorkspaceKey = "api_secrets"

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
}

var Fields = buildFields()

func ProviderSecretKey(provider string) (string, bool) {
	definition, ok := catalog.Lookup(provider)
	if !ok || (!definition.SupportsExactPreparedStations && !definition.SupportsEmbeddings) || len(definition.APIKeyEnvironmentKeys) == 0 {
		return "", false
	}
	if definition.ID == "azure" {
		return "azure_ai_api_key", true
	}
	return strings.ReplaceAll(definition.ID, "-", "_") + "_api_key", true
}

func buildFields() []Field {
	fields := make([]Field, 0)
	for _, definition := range catalog.ProductionDefinitions() {
		key, ok := ProviderSecretKey(definition.ID)
		if !ok {
			continue
		}
		description := definition.DisplayName + " model API access."
		if definition.ID == "openai" {
			description = "OpenAI model API access."
		}
		fields = append(fields, Field{
			Key:         key,
			Label:       definition.DisplayName + " API key",
			Description: description,
			EnvKeys:     append([]string(nil), definition.APIKeyEnvironmentKeys...),
		})
	}
	return fields
}

func ValidateStored(stored map[string]string) error {
	allowed := fieldKeys()
	for key, value := range stored {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("stored API secret field %q is unsupported or retired", key)
		}
		if value == "" || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("stored API secret field %q is not canonical", key)
		}
	}
	return nil
}

func MergeStored(stored map[string]string, updates map[string]string, clearKeys []string) (map[string]string, error) {
	if err := ValidateStored(stored); err != nil {
		return nil, err
	}
	if err := ValidateStored(updates); err != nil {
		return nil, err
	}
	allowed := fieldKeys()
	out := map[string]string{}
	for key, value := range stored {
		out[key] = value
	}
	seenClear := make(map[string]struct{}, len(clearKeys))
	for _, key := range clearKeys {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("API secret clear key %q is unsupported or retired", key)
		}
		if _, duplicate := seenClear[key]; duplicate {
			return nil, fmt.Errorf("API secret clear key %q is duplicated", key)
		}
		seenClear[key] = struct{}{}
		if _, conflict := updates[key]; conflict {
			return nil, fmt.Errorf("API secret field %q cannot be set and cleared together", key)
		}
		delete(out, key)
	}
	for key, value := range updates {
		out[key] = value
	}
	return out, nil
}

func fieldKeys() map[string]struct{} {
	allowed := make(map[string]struct{}, len(Fields))
	for _, field := range Fields {
		allowed[field.Key] = struct{}{}
	}
	return allowed
}

func MaskHint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "••••"
	}
	return "••••" + value[len(value)-4:]
}

func FieldList(stored map[string]string) []map[string]any {
	items := make([]map[string]any, 0, len(Fields))
	for _, field := range Fields {
		value := strings.TrimSpace(stored[field.Key])
		configured := value != ""
		source := "none"
		if configured {
			source = "database"
		}
		items = append(items, map[string]any{
			"key":         field.Key,
			"label":       field.Label,
			"description": field.Description,
			"env_keys":    field.EnvKeys,
			"configured":  configured,
			"source":      source,
			"hint":        MaskHint(value),
		})
	}
	return items
}
