package modelconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
	Options     []string `json:"options,omitempty"`
}

var Fields = []Field{
	{Key: "default_model", Label: "Default model", Description: "Primary conversation/responder model", EnvKeys: []string{"OMNI_MODEL", "OMNI_CONVERSATION_MODEL", "OLLAMA_MODEL_RESPONDER", "OLLAMA_MODEL"}},
	{Key: "fast_model", Label: "Fast model", Description: "Low-latency utility model for simple routing/tagging/fallbacks", EnvKeys: []string{"OMNI_FAST_MODEL", "OLLAMA_MODEL_FAST"}},
	{Key: "reasoning_model", Label: "Reasoning model", Description: "Deep reasoning model for analysis and complex planning", EnvKeys: []string{"OMNI_REASONING_MODEL", "OLLAMA_MODEL_REASONING"}},
	{Key: "planner_model", Label: "Planner model", Description: "Structured command planner", EnvKeys: []string{"OMNI_PLANNER_MODEL", "OMNI_STRUCTURED_PLANNER_MODEL", "OLLAMA_MODEL_PLANNER"}},
	{Key: "thinking_model", Label: "Thinking model", Description: "Internal thinking pilot channel", EnvKeys: []string{"OMNI_THINKING_MODEL", "OLLAMA_MODEL_THINKING", "OLLAMA_MODEL_REASONING"}},
	{Key: "analyzer_model", Label: "Analyzer model", Description: "Analysis and verification model", EnvKeys: []string{"OMNI_ANALYZER_MODEL", "OLLAMA_MODEL_ANALYZER"}},
	{Key: "responder_model", Label: "Responder model", Description: "Final response composition model", EnvKeys: []string{"OMNI_RESPONDER_MODEL", "OLLAMA_MODEL_RESPONDER"}},
	{Key: "tagger_model", Label: "Tagger model", Description: "Fast model for tags and classification", EnvKeys: []string{"OMNI_TAGGER_MODEL", "OLLAMA_MODEL_TAGGER"}},
	{Key: "search_model", Label: "Search model", Description: "Model used for search/research synthesis", EnvKeys: []string{"OMNI_SEARCH_MODEL", "OLLAMA_MODEL_SEARCH"}},
	{Key: "memory_model", Label: "Memory model", Description: "Model used for memory extraction and retrieval tasks", EnvKeys: []string{"OMNI_MEMORY_MODEL", "OLLAMA_MODEL_MEMORY"}},
	{Key: "evaluator_model", Label: "Evaluator model", Description: "Structured response evaluator", EnvKeys: []string{"OMNI_EVALUATOR_MODEL", "OLLAMA_MODEL_EVALUATOR"}},
	{Key: "executor_model", Label: "Executor model", Description: "Long-horizon code and tool execution specialist", EnvKeys: []string{"OMNI_EXECUTOR_MODEL", "OLLAMA_MODEL_SPECIALIST_SUBTASK_EXECUTOR"}},
	{Key: "shell_specialist_model", Label: "Shell specialist", Description: "Shell execution specialist", EnvKeys: []string{"OMNI_SHELL_SPECIALIST_MODEL", "OLLAMA_MODEL_SPECIALIST_SHELL_EXECUTION", "OLLAMA_MODEL_SHELL"}},
	{Key: "ollama_endpoint", Label: "Ollama endpoint", Description: "Ollama HTTP base URL", EnvKeys: []string{"OLLAMA_BASE_URL", "OMNI_OLLAMA_ENDPOINT"}},
}

type Config map[string]string

func FromEnv() Config {
	out := Config{}
	for _, field := range Fields {
		if value := lookupEnv(field.EnvKeys); value != "" {
			out[field.Key] = value
		}
	}
	return out
}

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
	allowed := modelConfigKeys()
	for key, rawValue := range payload {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("model config contains unsupported field %q", key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("model config field %q must be a string: %w", key, err)
		}
		if value = strings.TrimSpace(value); value != "" {
			out[key] = value
		}
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

func modelConfigKeys() map[string]struct{} {
	keys := make(map[string]struct{}, len(Fields))
	for _, field := range Fields {
		keys[field.Key] = struct{}{}
	}
	return keys
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

func Merge(layers ...Config) Config {
	out := Config{}
	for _, layer := range layers {
		for key, value := range layer {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	return out
}

func (c Config) Get(key string) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c[key])
}

func (c Config) ModelNames() []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, field := range Fields {
		if field.Key == "ollama_endpoint" {
			continue
		}
		value := c.Get(field.Key)
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

func (c Config) OllamaEndpoint(fallback string) string {
	if endpoint := c.Get("ollama_endpoint"); endpoint != "" {
		return endpoint
	}
	return strings.TrimSpace(fallback)
}

func (c Config) ToMap() map[string]string {
	out := map[string]string{}
	for key, value := range c {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func (c Config) FieldList(envValues map[string]string) []map[string]any {
	if envValues == nil {
		envValues = map[string]string{}
	}
	items := make([]map[string]any, 0, len(Fields))
	for _, field := range Fields {
		value := c.Get(field.Key)
		if value == "" {
			value = lookupMap(envValues, field.EnvKeys)
		}
		if value == "" {
			value = lookupEnv(field.EnvKeys)
		}
		items = append(items, map[string]any{
			"key":         field.Key,
			"label":       field.Label,
			"description": field.Description,
			"env_keys":    field.EnvKeys,
			"options":     field.Options,
			"value":       value,
		})
	}
	return items
}

func lookupEnv(keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func lookupMap(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}
