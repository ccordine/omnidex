package secrets

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func OverlayConfig(cfg *config.Config, resolver *Resolver) {
	OverlayConfigContext(context.Background(), cfg, resolver)
}

func OverlayConfigContext(ctx context.Context, cfg *config.Config, resolver *Resolver) {
	if cfg == nil || resolver == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, definition := range catalog.Definitions() {
		secretKey, ok := ProviderSecretKey(definition.ID)
		if !ok {
			continue
		}
		value := strings.TrimSpace(resolver.Get(ctx, secretKey))
		if value == "" {
			continue
		}
		switch definition.Protocol {
		case catalog.ProtocolOpenAICompatible:
			if cfg.CompatibleProviders == nil {
				cfg.CompatibleProviders = make(map[string]config.CompatibleProviderConfig)
			}
			providerConfig := cfg.CompatibleProviders[definition.ID]
			providerConfig.APIKey = value
			cfg.CompatibleProviders[definition.ID] = providerConfig
		case catalog.ProtocolAzure:
			cfg.AzureAIAPIKey = value
		case catalog.ProtocolGoogle:
			cfg.GoogleAPIKey = value
		case catalog.ProtocolAnthropic:
			cfg.AnthropicAPIKey = value
		case catalog.ProtocolHuggingFace:
			cfg.HuggingFaceAPIKey = value
		}
	}
}

func CodexAPIKey() string {
	if value := strings.TrimSpace(Lookup("codex_api_key")); value != "" {
		return value
	}
	return strings.TrimSpace(Lookup("openai_api_key"))
}
