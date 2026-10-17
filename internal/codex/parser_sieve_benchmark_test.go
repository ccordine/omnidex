package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/ollama"
)

// Only transport routing varies. Prompts, source, schemas, assertions and limits
// stay identical; a provider never receives fixture names or expected answers.
func installedSieveProvider(t *testing.T) (codingagent.Provider, codingagent.Route) {
	t.Helper()
	client, model := installedAgentClient(t)
	client.timeout = 45 * time.Second
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
		parser, err := ollama.NewParser(os.Getenv("OMNI_TEST_OLLAMA_PARSER_URL"), 45*time.Second, 8192)
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
	Step    codingagent.ParserStep `json:"step"`
	Prompt  string                 `json:"prompt"`
	Context string                 `json:"context"`
}

// These are code-rendered envelopes, explicitly separate from original provider
// receipts. Capturing them does not alter the actual provider call or its input.
func sieveBenchmarkEnvelopeFor(call codingagent.Assignment, frame json.RawMessage) (*sieveBenchmarkEnvelope, error) {
	var receipt struct {
		Processor string                 `json:"processor"`
		Stage     string                 `json:"stage"`
		Value     codingagent.ParserStep `json:"value"`
	}
	var header struct{ Processor, Stage string }
	if err := json.Unmarshal(frame, &header); err != nil {
		return nil, err
	}
	if header.Processor != "response_sieve" || header.Stage != "leaf_opened" {
		return nil, nil
	}
	if err := json.Unmarshal(frame, &receipt); err != nil {
		return nil, err
	}
	call.ParserStep, call.Context = &receipt.Value, ""
	prompt, err := codingagent.RenderParserPrompt(call)
	if err != nil {
		return nil, fmt.Errorf("capture parser prompt: %w", err)
	}
	context, err := codingagent.RenderParserContext(call)
	if err != nil {
		return nil, fmt.Errorf("capture parser context: %w", err)
	}
	return &sieveBenchmarkEnvelope{Step: receipt.Value, Prompt: prompt, Context: context}, nil
}
