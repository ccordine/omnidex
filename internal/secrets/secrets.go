package secrets

import (
	"os"
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
	if !ok || len(definition.APIKeyEnvironmentKeys) == 0 {
		return "", false
	}
	if definition.ID == "azure" {
		return "azure_ai_api_key", true
	}
	return strings.ReplaceAll(definition.ID, "-", "_") + "_api_key", true
}

func buildFields() []Field {
	fields := []Field{
		{Key: "cursor_api_key", Label: "Cursor API key", Description: "Cursor SDK architect delegation.", EnvKeys: []string{"CURSOR_API_KEY"}},
		{Key: "codex_api_key", Label: "Codex API key", Description: "Codex SDK architect delegation. Falls back to OpenAI key when unset.", EnvKeys: []string{"CODEX_API_KEY"}},
	}
	for _, definition := range catalog.Definitions() {
		key, ok := ProviderSecretKey(definition.ID)
		if !ok {
			continue
		}
		description := definition.DisplayName + " model API access."
		if definition.ID == "openai" {
			description = "OpenAI API access. Also used by Codex when no dedicated Codex key is set."
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

func LookupEnv(keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func MergeStored(stored map[string]string, updates map[string]string, clearKeys []string) map[string]string {
	out := map[string]string{}
	for key, value := range stored {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	for _, key := range clearKeys {
		delete(out, strings.TrimSpace(key))
	}
	for key, value := range updates {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
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
		if value == "" {
			value = LookupEnv(field.EnvKeys)
		}
		configured := value != ""
		source := "none"
		if strings.TrimSpace(stored[field.Key]) != "" {
			source = "database"
		} else if configured {
			source = "environment"
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
