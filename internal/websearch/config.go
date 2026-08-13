package websearch

import (
	"fmt"
	"math"
	"strings"
)

func validateConfig(config Config) error {
	if err := validateHardConfigBounds(config); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if len(config.Providers) == 0 {
		return fmt.Errorf("%w: at least one provider is required", ErrInvalidConfig)
	}
	seen := make(map[ProviderID]struct{}, len(config.Providers))
	for _, provider := range config.Providers {
		if _, ok := providerDefinitionFor(provider); !ok {
			return fmt.Errorf("%w: unsupported provider %q", ErrInvalidConfig, provider)
		}
		if _, duplicate := seen[provider]; duplicate {
			return fmt.Errorf("%w: duplicate provider %q", ErrInvalidConfig, provider)
		}
		seen[provider] = struct{}{}
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfig)
	}
	if config.PerDocumentBytes < 1 {
		return fmt.Errorf("%w: per-document byte budget must be positive", ErrInvalidConfig)
	}
	if config.MaxDocuments < 1 {
		return fmt.Errorf("%w: max documents must be positive", ErrInvalidConfig)
	}
	if config.PerDocumentBytes > math.MaxInt/config.MaxDocuments ||
		config.TotalDocumentBytes < config.PerDocumentBytes*config.MaxDocuments {
		return fmt.Errorf("%w: total document budget must cover every explicitly fetchable document", ErrInvalidConfig)
	}
	if config.MaxCandidatesPerProvider < 1 {
		return fmt.Errorf("%w: max candidates per provider must be positive", ErrInvalidConfig)
	}
	if config.MaxCandidates < config.MaxCandidatesPerProvider || config.MaxCandidates < config.MaxDocuments {
		return fmt.Errorf("%w: max candidates must cover the provider and document bounds", ErrInvalidConfig)
	}
	if config.MaxResponseBytes < int64(config.PerDocumentBytes) {
		return fmt.Errorf("%w: response byte bound must cover one document budget", ErrInvalidConfig)
	}
	return nil
}

func validateQuery(request QueryRequest) (string, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return "", fmt.Errorf("%w: query is empty", ErrInvalidQuery)
	}
	if len(query) > 1_024 {
		return "", fmt.Errorf("%w: query exceeds 1024 bytes", ErrInvalidQuery)
	}
	return query, nil
}
