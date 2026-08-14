package llmprovider

import (
	"context"
	"fmt"
	"strings"
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

func (resolver *lazyExactStationResolver) RequireExactPreparedContract() error {
	if resolver == nil {
		return fmt.Errorf("lazy exact station resolver is uninitialized")
	}
	return nil
}

func (resolver *lazyExactStationResolver) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	client, err := resolver.resolve()
	if err != nil {
		return err
	}
	return client.ValidateExactPreparedProvider(expected)
}

func (resolver *lazyExactStationResolver) ValidateExactPreparedContract(
	prepared llm.PreparedModel,
) error {
	client, err := resolver.resolve()
	if err != nil {
		return err
	}
	return client.ValidateExactPreparedContract(prepared)
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

func (resolver *lazyExactStationResolver) DiscoverProviderIdentityEvidence(
	ctx context.Context,
	selection llm.ProviderIdentitySelection,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	client, err := resolver.resolve()
	if err == nil {
		return client.DiscoverProviderIdentityEvidence(ctx, selection, challenge)
	}
	evidence, evidenceErr := llm.NewUndispatchedProviderIdentityEvidence(selection)
	if evidenceErr != nil {
		return llm.ObservedProviderIdentity{}, fmt.Errorf(
			"resolve exact station provider authority: %v; construct undispatched evidence: %w",
			err, evidenceErr,
		)
	}
	return llm.ObservedProviderIdentity{Evidence: evidence}, fmt.Errorf(
		"resolve exact station provider authority: %w", err,
	)
}

func (resolver *lazyExactStationResolver) resolve() (llm.ExactStationClient, error) {
	if resolver == nil {
		return nil, fmt.Errorf("lazy exact station resolver is uninitialized")
	}
	resolver.once.Do(func() {
		provider := strings.TrimSpace(resolver.cfg.LLMProvider)
		if provider == "" {
			resolver.err = fmt.Errorf("LLM_PROVIDER is not configured")
			return
		}
		definition, ok := catalog.Lookup(provider)
		if !ok || !definition.SupportsExactPreparedStations {
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
		resolver.client, resolver.err = newExactStationProvider(
			resolver.cfg, definition, resolver.cfg.RequestTimeout,
		)
		if resolver.err == nil {
			resolver.err = resolver.client.RequireExactPreparedContract()
		}
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
		provider := strings.TrimSpace(resolver.cfg.EmbeddingProvider)
		if provider == "" {
			resolver.err = fmt.Errorf("EMBEDDING_PROVIDER is not configured")
			return
		}
		definition, ok := catalog.Lookup(provider)
		if !ok || !definition.SupportsEmbeddings {
			resolver.err = fmt.Errorf("embedding provider %q is unsupported", provider)
			return
		}
		if err := config.ValidateProviderConfiguration(
			resolver.cfg, definition.ID, "EMBEDDING_PROVIDER",
		); err != nil {
			resolver.err = err
			return
		}
		resolver.client, resolver.err = newEmbeddingProvider(
			resolver.cfg, definition, resolver.cfg.EmbeddingModel, resolver.cfg.RequestTimeout,
		)
	})
	return resolver.client, resolver.err
}
