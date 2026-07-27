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
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
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
	generationDefinition, ok := catalog.Lookup(cfg.LLMProvider)
	if !ok || !generationDefinition.SupportsGeneration {
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.LLMProvider)
	}
	embeddingDefinition, ok := catalog.Lookup(cfg.EmbeddingProvider)
	if !ok || !embeddingDefinition.SupportsEmbeddings {
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.EmbeddingProvider)
	}
	generation, err := NewProvider(cfg, Options{
		Provider: generationDefinition.ID,
		Model:    cfg.DefaultModel,
		Timeout:  cfg.RequestTimeout,
	})
	if err != nil {
		return nil, err
	}
	embedding, err := NewProvider(cfg, Options{
		Provider:       embeddingDefinition.ID,
		Model:          cfg.DefaultModel,
		EmbeddingModel: cfg.EmbeddingModel,
		Timeout:        cfg.RequestTimeout,
	})
	if err != nil {
		return nil, err
	}
	if generationDefinition.ID == embeddingDefinition.ID {
		return generation, nil
	}
	return llm.NewRoutedClient(generation, embedding), nil
}

func NewProvider(cfg config.Config, opts Options) (llm.Client, error) {
	definition, ok := catalog.Lookup(opts.Provider)
	if !ok || !definition.SupportsGeneration {
		return nil, fmt.Errorf("unsupported LLM provider: %s", opts.Provider)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = cfg.RequestTimeout
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	model := resolvedGenerationModel(definition, cfg, opts.Model)
	embeddingModel := resolvedEmbeddingModel(definition, cfg, opts.EmbeddingModel)

	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return ollama.New(cfg.OllamaBaseURL, model, embeddingModel, timeout, cfg.InferenceContextTokens), nil
	case catalog.ProtocolOpenAICompatible:
		providerConfig, configured := cfg.CompatibleProviders[definition.ID]
		if !configured {
			return nil, fmt.Errorf("compatible provider configuration is missing for %s", definition.ID)
		}
		apiKeyName := "API key"
		if len(definition.APIKeyEnvironmentKeys) > 0 {
			apiKeyName = definition.APIKeyEnvironmentKeys[0]
		}
		return openai.NewCompatible(
			definition.ID,
			apiKeyName,
			providerConfig.BaseURL,
			providerConfig.APIKey,
			model,
			embeddingModel,
			providerConfig.Organization,
			providerConfig.Project,
			timeout,
		)
	case catalog.ProtocolAzure:
		return openai.NewAzureAI(cfg.AzureAIBaseURL, cfg.AzureAIAPIKey, model, embeddingModel, cfg.AzureAIAPIVersion, cfg.AzureAIAPIStyle, timeout), nil
	case catalog.ProtocolGoogle:
		return googleai.New(cfg.GoogleBaseURL, cfg.GoogleAPIKey, model, embeddingModel, timeout), nil
	case catalog.ProtocolAnthropic:
		return anthropic.New(cfg.AnthropicBaseURL, cfg.AnthropicAPIKey, model, cfg.AnthropicVersion, cfg.AnthropicMaxTokens, timeout), nil
	case catalog.ProtocolHuggingFace:
		return huggingface.New(cfg.HuggingFaceBaseURL, cfg.HuggingFaceAPIKey, model, embeddingModel, timeout), nil
	default:
		return nil, fmt.Errorf("provider %s uses unsupported protocol %s", definition.ID, definition.Protocol)
	}
}

func resolvedGenerationModel(definition catalog.Definition, cfg config.Config, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if configuredProvider, ok := catalog.Lookup(cfg.LLMProvider); ok && configuredProvider.ID == definition.ID {
		if model := strings.TrimSpace(cfg.DefaultModel); model != "" {
			return model
		}
	}
	if providerModels, ok := cfg.ProviderModels[definition.ID]; ok {
		if model := strings.TrimSpace(providerModels.Default); model != "" {
			return model
		}
	}
	return strings.TrimSpace(definition.DefaultModel)
}

func resolvedEmbeddingModel(definition catalog.Definition, cfg config.Config, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if configuredProvider, ok := catalog.Lookup(cfg.EmbeddingProvider); ok && configuredProvider.ID == definition.ID {
		if model := strings.TrimSpace(cfg.EmbeddingModel); model != "" {
			return model
		}
	}
	if providerModels, ok := cfg.ProviderModels[definition.ID]; ok {
		if model := strings.TrimSpace(providerModels.Embedding); model != "" {
			return model
		}
	}
	return strings.TrimSpace(definition.DefaultEmbeddingModel)
}
