package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/ollama"
)

// Only transport routing varies. Prompts, source, schemas, assertions and limits
// stay identical; a provider never receives fixture names or expected answers.
func installedSieveProvider(t *testing.T) (codingagent.Provider, codingagent.Route) {
	t.Helper()
	client, model := installedAgentClient(t)
	provider := os.Getenv("OMNI_TEST_PARSER_PROVIDER")
	if provider == "" {
		provider = "codex"
	}
	switch provider {
	case "codex":
		effort := os.Getenv("OMNI_TEST_CODEX_EFFORT")
		if effort == "" {
			effort = "low"
		}
		return client, codingagent.Route{Provider: provider, Model: model, Effort: effort, ServiceTier: "default"}
	case "ollama":
		model := os.Getenv("OMNI_TEST_OLLAMA_PARSER_MODEL")
		if model == "" {
			t.Fatal("OMNI_TEST_OLLAMA_PARSER_MODEL is required for the Ollama corpus route")
		}
		parser, err := ollama.NewParser(os.Getenv("OMNI_TEST_OLLAMA_PARSER_URL"), 8192)
		if err != nil {
			t.Fatal(err)
		}
		return parser, codingagent.Route{Provider: provider, Model: model}
	default:
		t.Fatalf("unsupported corpus provider %q", provider)
		return nil, codingagent.Route{}
	}
}

type sieveBenchmarkEnvelope struct {
	Prompt  string `json:"prompt"`
	Context string `json:"context"`
}

// Render the same complete-response envelope used by the production transport.
func sieveBenchmarkEnvelopeFor(call codingagent.Assignment, frame json.RawMessage) (*sieveBenchmarkEnvelope, error) {
	prompt, err := codingagent.RenderParserPrompt(call)
	if err != nil {
		return nil, fmt.Errorf("capture parser prompt: %w", err)
	}
	background, err := codingagent.RenderParserContext(call)
	if err != nil {
		return nil, fmt.Errorf("capture parser context: %w", err)
	}
	return &sieveBenchmarkEnvelope{Prompt: prompt, Context: background}, nil
}
