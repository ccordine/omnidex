package llmprovider

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/googleai"
	"github.com/gryph/omnidex/internal/huggingface"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/openai"
)

type Transports struct {
	Stations   llm.ExactStationClient
	Embeddings llm.EmbeddingClient
}

func newExactStationProvider(
	cfg config.Config,
	definition catalog.Definition,
	timeout time.Duration,
) (llm.ExactStationClient, error) {
	if !definition.SupportsExactPreparedStations {
		return nil, fmt.Errorf(
			"provider %q does not implement the exact prepared station contract",
			definition.ID,
		)
	}
	timeout = providerTimeout(timeout, cfg.RequestTimeout)
	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return ollama.New(
			cfg.OllamaBaseURL, "", "", timeout, cfg.InferenceContextTokens,
		), nil
	default:
		return nil, fmt.Errorf(
			"provider %q advertises an unimplemented exact station protocol %q",
			definition.ID, definition.Protocol,
		)
	}
}

func newEmbeddingProvider(
	cfg config.Config,
	definition catalog.Definition,
	requestedModel string,
	timeout time.Duration,
) (llm.EmbeddingClient, error) {
	if !definition.SupportsEmbeddings {
		return nil, fmt.Errorf("provider %q does not implement embeddings", definition.ID)
	}
	model := resolvedEmbeddingModel(definition, cfg, requestedModel)
	if model == "" {
		return nil, fmt.Errorf("embedding model is required for provider %q", definition.ID)
	}
	timeout = providerTimeout(timeout, cfg.RequestTimeout)

	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return ollama.New(
			cfg.OllamaBaseURL, "", model, timeout, cfg.InferenceContextTokens,
		), nil
	case catalog.ProtocolOpenAICompatible:
		providerConfig, configured := cfg.CompatibleProviders[definition.ID]
		if !configured {
			return nil, fmt.Errorf(
				"compatible provider configuration is missing for %s", definition.ID,
			)
		}
		apiKeyName := "API key"
		if len(definition.APIKeyEnvironmentKeys) > 0 {
			apiKeyName = definition.APIKeyEnvironmentKeys[0]
		}
		return openai.NewCompatibleEmbedding(
			definition.ID, apiKeyName, providerConfig.BaseURL, providerConfig.APIKey,
			model, providerConfig.Organization, providerConfig.Project, timeout,
		)
	case catalog.ProtocolAzure:
		return openai.NewAzureAIEmbedding(
			cfg.AzureAIBaseURL, cfg.AzureAIAPIKey, model, cfg.AzureAIAPIVersion,
			cfg.AzureAIAPIStyle, timeout,
		)
	case catalog.ProtocolGoogle:
		return googleai.NewEmbedding(
			cfg.GoogleBaseURL, cfg.GoogleAPIKey, model, timeout,
		), nil
	case catalog.ProtocolHuggingFace:
		return huggingface.NewEmbedding(
			cfg.HuggingFaceBaseURL, cfg.HuggingFaceAPIKey, model, timeout,
		), nil
	default:
		return nil, fmt.Errorf(
			"provider %s uses unsupported embedding protocol %s",
			definition.ID, definition.Protocol,
		)
	}
}

func providerTimeout(requested, configured time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	if configured > 0 {
		return configured
	}
	return 90 * time.Second
}

func resolvedEmbeddingModel(
	definition catalog.Definition,
	cfg config.Config,
	requested string,
) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if configuredProvider, ok := catalog.Lookup(cfg.EmbeddingProvider); ok &&
		configuredProvider.ID == definition.ID {
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
