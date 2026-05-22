package llmprovider

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/anthropic"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/googleai"
	"github.com/gryph/omnidex/internal/huggingface"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/openai"
)

type Options struct {
	Provider       string
	Model          string
	EmbeddingModel string
	Timeout        time.Duration
}

func NewFromConfig(cfg config.Config) (llm.Client, error) {
	generation, err := NewProvider(cfg, Options{
		Provider: cfg.LLMProvider,
		Model:    cfg.DefaultModel,
		Timeout:  cfg.RequestTimeout,
	})
	if err != nil {
		return nil, err
	}
	embedding, err := NewProvider(cfg, Options{
		Provider:       cfg.EmbeddingProvider,
		Model:          cfg.DefaultModel,
		EmbeddingModel: cfg.EmbeddingModel,
		Timeout:        cfg.RequestTimeout,
	})
	if err != nil {
		return nil, err
	}
	if normalizeProvider(cfg.LLMProvider) == normalizeProvider(cfg.EmbeddingProvider) {
		return generation, nil
	}
	return llm.NewRoutedClient(generation, embedding), nil
}

func NewProvider(cfg config.Config, opts Options) (llm.Client, error) {
	provider := normalizeProvider(opts.Provider)
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = cfg.RequestTimeout
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = cfg.DefaultModel
	}
	embeddingModel := strings.TrimSpace(opts.EmbeddingModel)
	if embeddingModel == "" {
		embeddingModel = cfg.EmbeddingModel
	}

	switch provider {
	case "ollama":
		return ollama.New(cfg.OllamaBaseURL, model, embeddingModel, timeout), nil
	case "openai":
		return openai.New(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, model, embeddingModel, cfg.OpenAIOrganization, cfg.OpenAIProject, timeout), nil
	case "azure":
		return openai.NewAzureAI(cfg.AzureAIBaseURL, cfg.AzureAIAPIKey, model, embeddingModel, cfg.AzureAIAPIVersion, cfg.AzureAIAPIStyle, timeout), nil
	case "xai":
		return openai.NewCompatible("xai", "XAI_API_KEY", cfg.XAIBaseURL, cfg.XAIAPIKey, model, "", "", "", timeout), nil
	case "google":
		return googleai.New(cfg.GoogleBaseURL, cfg.GoogleAPIKey, model, embeddingModel, timeout), nil
	case "anthropic":
		return anthropic.New(cfg.AnthropicBaseURL, cfg.AnthropicAPIKey, model, cfg.AnthropicVersion, cfg.AnthropicMaxTokens, timeout), nil
	case "huggingface":
		return huggingface.New(cfg.HuggingFaceBaseURL, cfg.HuggingFaceAPIKey, model, embeddingModel, timeout), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", opts.Provider)
	}
}

func normalizeProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai", "chatgpt", "chat-gpt":
		return "openai"
	case "azure", "azureai", "azure-ai", "azure-openai", "azure_openai", "microsoft", "msai", "windows", "windowsai", "windows-ai":
		return "azure"
	case "xai", "x-ai", "grok", "grock":
		return "xai"
	case "google", "gemini", "googleai", "google-ai":
		return "google"
	case "anthropic", "claude":
		return "anthropic"
	case "huggingface", "hugging-face", "hf":
		return "huggingface"
	default:
		return "ollama"
	}
}
