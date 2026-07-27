package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/omni"
)

type recordingWorkerLLM struct {
	model  string
	prompt string
	output string
}

func (c *recordingWorkerLLM) Generate(_ context.Context, model, prompt string) (string, error) {
	c.model = model
	c.prompt = prompt
	return c.output, nil
}

func (*recordingWorkerLLM) PrepareContextModel(context.Context, string, string) (llm.PreparedModel, error) {
	return llm.PreparedModel{}, nil
}

func (*recordingWorkerLLM) GeneratePrepared(context.Context, llm.PreparedModel) (string, error) {
	return "", nil
}

func (*recordingWorkerLLM) CleanupPreparedModel(llm.PreparedModel) {}

func (*recordingWorkerLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (*recordingWorkerLLM) SuggestTags(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (*recordingWorkerLLM) SuggestTagsWithModel(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}

func TestDataSourceLLMClientUsesConfiguredClientAndTaggingModel(t *testing.T) {
	client := &recordingWorkerLLM{output: `{"answer":"ok"}`}
	service := &Service{llm: client, models: ModelRouting{Tagging: "provider-model"}}

	adapter, err := service.dataSourceLLMClient()
	if err != nil {
		t.Fatalf("dataSourceLLMClient() error: %v", err)
	}
	response, err := adapter.ChatRaw(context.Background(), omni.OllamaChatRequest{
		Messages: []omni.OllamaMessage{
			{Role: "system", Content: "Return JSON only."},
			{Role: "user", Content: "Analyze this."},
		},
		Format:  map[string]any{"type": "object"},
		Options: map[string]any{"temperature": 0, "num_predict": 200},
	})
	if err != nil {
		t.Fatalf("ChatRaw() error: %v", err)
	}
	if response.Content != client.output || response.Model != "provider-model" {
		t.Fatalf("ChatRaw() response=%#v", response)
	}
	if client.model != "provider-model" {
		t.Fatalf("Generate() model=%q want provider-model", client.model)
	}
	for _, required := range []string{"SYSTEM:", "USER:", "RESPONSE CONTRACT:", "200 tokens"} {
		if !strings.Contains(client.prompt, required) {
			t.Errorf("Generate() prompt missing %q: %s", required, client.prompt)
		}
	}
}

func TestDataSourcePromptRejectsProviderSpecificOrUnknownState(t *testing.T) {
	for _, request := range []omni.OllamaChatRequest{
		{Messages: []omni.OllamaMessage{{Role: "mystery", Content: "x"}}},
		{Messages: []omni.OllamaMessage{{Role: "user", Content: "x"}}, KeepAlive: "5m"},
		{Messages: []omni.OllamaMessage{{Role: "user", Content: "x"}}, Options: map[string]any{"seed": 42}},
		{Messages: []omni.OllamaMessage{{Role: "user", Content: "x"}}, Options: map[string]any{"temperature": 0.8}},
	} {
		if _, err := dataSourcePrompt(request); err == nil {
			t.Fatalf("dataSourcePrompt() accepted invalid request %#v", request)
		}
	}
}
