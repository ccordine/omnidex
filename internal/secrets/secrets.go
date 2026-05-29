package secrets

import (
	"os"
	"strings"
)

const WorkspaceKey = "api_secrets"

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
}

var Fields = []Field{
	{Key: "openai_api_key", Label: "OpenAI API key", Description: "OpenAI-compatible API access. Also used by Codex when no dedicated Codex key is set.", EnvKeys: []string{"OPENAI_API_KEY"}},
	{Key: "cursor_api_key", Label: "Cursor API key", Description: "Cursor SDK architect delegation.", EnvKeys: []string{"CURSOR_API_KEY"}},
	{Key: "codex_api_key", Label: "Codex API key", Description: "Codex SDK architect delegation. Falls back to OpenAI key when unset.", EnvKeys: []string{"CODEX_API_KEY"}},
	{Key: "anthropic_api_key", Label: "Anthropic API key", Description: "Claude models via Anthropic API.", EnvKeys: []string{"ANTHROPIC_API_KEY"}},
	{Key: "google_api_key", Label: "Google / Gemini API key", Description: "Gemini models via Google AI.", EnvKeys: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}},
	{Key: "xai_api_key", Label: "xAI / Grok API key", Description: "Grok models via xAI.", EnvKeys: []string{"XAI_API_KEY", "GROK_API_KEY"}},
	{Key: "azure_ai_api_key", Label: "Azure AI API key", Description: "Azure OpenAI / Foundry deployments.", EnvKeys: []string{"AZURE_AI_API_KEY", "AZURE_OPENAI_API_KEY"}},
	{Key: "huggingface_api_key", Label: "Hugging Face API key", Description: "Hugging Face Inference Providers.", EnvKeys: []string{"HUGGINGFACE_API_KEY", "HF_TOKEN"}},
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
