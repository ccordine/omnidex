package llmprovider

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

type lazyExactStationResolver struct {
	cfg    config.Config
	once   sync.Once
	client llm.ExactStationClient
	err    error
}

type lazyEmbeddingResolver struct {
	cfg    config.Config
	once   sync.Once
	client llm.EmbeddingClient
	err    error
}

// NewLazyFromConfig freezes configuration but performs no provider validation,
// construction, discovery, or I/O. Each authority resolves at its first actual
// provider operation.
func NewLazyFromConfig(cfg config.Config) Transports {
	cfg.CompatibleProviders = config.CloneCompatibleProviders(cfg.CompatibleProviders)
	cfg.ProviderModels = config.CloneProviderModels(cfg.ProviderModels)
	return Transports{
		Stations:   &lazyExactStationResolver{cfg: cfg},
		Embeddings: &lazyEmbeddingResolver{cfg: cfg},
	}
}

func (resolver *lazyExactStationResolver) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client, err := resolver.resolve()
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return client.GeneratePreparedExact(ctx, prepared)
}

func (resolver *lazyExactStationResolver) resolve() (llm.ExactStationClient, error) {
	if resolver == nil {
		return nil, fmt.Errorf("lazy exact station resolver is uninitialized")
	}
	resolver.once.Do(func() {
		provider := resolver.cfg.LLMProvider
		if provider == "" {
			resolver.err = fmt.Errorf("LLM_PROVIDER is not configured")
			return
		}
		definition, err := catalog.Resolve(provider)
		if err != nil {
			resolver.err = fmt.Errorf("resolve LLM provider %q: %w", provider, err)
			return
		}
		if !definition.SupportsExactPreparedStations {
			resolver.err = fmt.Errorf(
				"LLM provider %q does not implement the exact prepared station contract", provider,
			)
			return
		}
		if err := config.ValidateProviderConfiguration(
			resolver.cfg, definition.ID, "LLM_PROVIDER",
		); err != nil {
			resolver.err = err
			return
		}
		resolver.client, resolver.err = newExactStationProvider(resolver.cfg, definition)
	})
	return resolver.client, resolver.err
}

func (resolver *lazyEmbeddingResolver) Embedding(
	ctx context.Context,
	content string,
) ([]float64, error) {
	client, err := resolver.resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve embedding provider authority: %w", err)
	}
	return client.Embedding(ctx, content)
}

func (resolver *lazyEmbeddingResolver) resolve() (llm.EmbeddingClient, error) {
	if resolver == nil {
		return nil, fmt.Errorf("lazy embedding resolver is uninitialized")
	}
	resolver.once.Do(func() {
		provider := resolver.cfg.EmbeddingProvider
		if provider == "" {
			resolver.err = fmt.Errorf("EMBEDDING_PROVIDER is not configured")
			return
		}
		definition, err := catalog.Resolve(provider)
		if err != nil {
			resolver.err = fmt.Errorf("resolve embedding provider %q: %w", provider, err)
			return
		}
		if !definition.SupportsEmbeddings {
			resolver.err = fmt.Errorf("embedding provider %q is unsupported", provider)
			return
		}
		if err := config.ValidateProviderConfiguration(
			resolver.cfg, definition.ID, "EMBEDDING_PROVIDER",
		); err != nil {
			resolver.err = err
			return
		}
		resolver.client, resolver.err = newEmbeddingProvider(resolver.cfg, definition)
	})
	return resolver.client, resolver.err
}
