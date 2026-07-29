package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestLLMResponseContractIsSelectedByInternalJobType(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		format     string
		maxTokens  int
		promptHint string
	}{
		{
			name:       "portable semantic station",
			scope:      "portable_semantic_worker",
			format:     llm.ResponseFormatJSON,
			maxTokens:  1024,
			promptHint: "Return only one JSON object that satisfies the supplied response contract.",
		},
		{
			name:       "portable fragment station",
			scope:      "portable_fragment_worker",
			format:     "",
			maxTokens:  4096,
			promptHint: "Return only the raw TypeScript function required by the supplied contract. No JSON, Markdown, import, export, path, or commentary.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract, err := llmResponseContractForScope(test.scope)
			if err != nil {
				t.Fatal(err)
			}
			if contract.Format != test.format || contract.MaxTokens != test.maxTokens || contract.PromptHint != test.promptHint {
				t.Fatalf("llmResponseContractForScope(%q)=%#v", test.scope, contract)
			}
		})
	}
}

func TestLLMResponseContractRejectsUnregisteredScope(t *testing.T) {
	if _, err := llmResponseContractForScope("legacy_guessing"); err == nil {
		t.Fatal("unregistered LLM scope was accepted")
	}
}
