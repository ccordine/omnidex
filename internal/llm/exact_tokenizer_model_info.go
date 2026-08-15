package llm

import (
	"encoding/json"
	"strings"
)

func (profile exactProviderModelProfile) matchesExactTokenizerFields(
	info map[string]json.RawMessage,
) bool {
	if len(profile.exactTokenizerFields) == 0 {
		return validateExactTokenizerPayloads(info) == nil
	}
	for key, want := range profile.exactTokenizerFields {
		raw, exists := info[key]
		if !exists || string(raw) != want {
			return false
		}
	}
	for key := range info {
		if !strings.HasPrefix(key, "tokenizer.ggml.") ||
			key == "tokenizer.ggml.model" || key == "tokenizer.ggml.pre" {
			continue
		}
		if _, allowed := profile.explicitAdd[key]; allowed {
			continue
		}
		if _, allowed := profile.exactTokenizerFields[key]; !allowed {
			return false
		}
	}
	return true
}
