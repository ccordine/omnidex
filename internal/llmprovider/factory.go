package llmprovider

import (
	"fmt"

	"github.com/gryph/omnidex/internal/codex"
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
) (llm.ExactStationClient, error) {
	if !definition.SupportsExactPreparedStations {
		return nil, fmt.Errorf(
			"provider %q does not implement the exact prepared station contract",
			definition.ID,
		)
	}
	switch definition.Protocol {
	case catalog.ProtocolCodex:
		return codex.New(cfg.CodexCommand, cfg.CodexDirectory)
	case catalog.ProtocolOllama:
		return ollama.New(
			cfg.OllamaBaseURL, "", "",
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
) (llm.EmbeddingClient, error) {
	if !definition.SupportsEmbeddings {
		return nil, fmt.Errorf("provider %q does not implement embeddings", definition.ID)
	}
	model := cfg.EmbeddingModel
	if model == "" {
		return nil, fmt.Errorf("embedding model is required for provider %q", definition.ID)
	}

	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return ollama.New(
			cfg.OllamaBaseURL, "", model,
		), nil
	case catalog.ProtocolOpenAICompatible:
		providerConfig, configured := cfg.CompatibleProviders[definition.ID]
		if !configured {
			return nil, fmt.Errorf(
				"compatible provider configuration is missing for %s", definition.ID,
			)
		}
		apiKeyName := definition.EnvironmentKey("API_KEY")
		return openai.NewCompatibleEmbedding(
			definition.ID, apiKeyName, providerConfig.BaseURL, providerConfig.APIKey,
			model, providerConfig.Organization, providerConfig.Project,
		)
	case catalog.ProtocolAzure:
		return openai.NewAzureAIEmbedding(
			cfg.AzureAIBaseURL, cfg.AzureAIAPIKey, model, cfg.AzureAIAPIVersion,
			cfg.AzureAIAPIStyle,
		)
	case catalog.ProtocolGoogle:
		return googleai.NewEmbedding(
			cfg.GoogleBaseURL, cfg.GoogleAPIKey, model,
		), nil
	case catalog.ProtocolHuggingFace:
		return huggingface.NewEmbedding(
			cfg.HuggingFaceBaseURL, cfg.HuggingFaceAPIKey, model,
		), nil
	default:
		return nil, fmt.Errorf(
			"provider %s uses unsupported embedding protocol %s",
			definition.ID, definition.Protocol,
		)
	}
}
