package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/config"
)

func TestResolvePersonaLLMSupportsConfiguredChineseProvider(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:  "openai",
			DefaultModel: "gpt-test",
			ProviderModels: map[string]config.ProviderModelConfig{
				"openai": {Default: "gpt-test"},
				"qwen":   {Default: "qwen-current"},
			},
			CompatibleProviders: map[string]config.CompatibleProviderConfig{
				"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "openai-key"},
				"qwen":   {BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: "qwen-key"},
			},
		},
		RequestTimeout: time.Second,
	})

	resolved, err := server.resolvePersonaLLM(personaRequest{LLM: &personaLLMRequest{Provider: "dashscope"}})
	if err != nil {
		t.Fatalf("resolvePersonaLLM() error: %v", err)
	}
	if resolved.Provider != "qwen" || resolved.Model != "qwen-current" {
		t.Fatalf("resolved provider/model=%q/%q", resolved.Provider, resolved.Model)
	}
}

func TestDecodePersonaRequestRejectsRemovedOpenAIFallbackPayload(t *testing.T) {
	body := []byte(`{"prompt":"hello","llm":{"provider":"openai","openai":{"use_server_fallback":true}}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	if _, ok := decodePersonaRequest(recorder, req); ok {
		t.Fatal("legacy llm.openai fallback payload was accepted")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestResolvePersonaLLMRejectsUnknownProviderAndListsCatalogProviders(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{LLMProvider: "ollama", DefaultModel: "llama3.2"},
	})

	_, err := server.resolvePersonaLLM(personaRequest{LLM: &personaLLMRequest{Provider: "invented-provider"}})
	if err == nil {
		t.Fatal("expected unknown provider failure")
	}
	for _, provider := range []string{"deepseek", "qwen", "moonshot", "zhipu", "minimax", "qianfan", "hunyuan", "doubao"} {
		if !strings.Contains(err.Error(), provider) {
			t.Errorf("provider error missing %q: %v", provider, err)
		}
	}
}

func TestPersonaCompatibleOverrideNeverUsesServerCredentialImplicitly(t *testing.T) {
	server := NewServerWithOptions(nil, &fakeLLMClient{}, ServerOptions{
		ProviderConfig: config.Config{
			LLMProvider:  "qwen",
			DefaultModel: "qwen-current",
			ProviderModels: map[string]config.ProviderModelConfig{
				"qwen": {Default: "qwen-current"},
			},
			CompatibleProviders: map[string]config.CompatibleProviderConfig{
				"qwen": {BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: "server-key"},
			},
		},
	})

	_, err := server.resolvePersonaLLM(personaRequest{LLM: &personaLLMRequest{
		Provider: "qwen",
		Compatible: &personaCompatibleConfig{
			BaseURL: "https://workspace.example/compatible-mode/v1",
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "QWEN_API_KEY") {
		t.Fatalf("resolvePersonaLLM() error=%v, want explicit credential failure", err)
	}
}
