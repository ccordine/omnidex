package secrets

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/config"
)

func OverlayConfig(cfg *config.Config, resolver *Resolver) {
	if cfg == nil || resolver == nil {
		return
	}
	ctx := context.Background()
	if value := resolver.Get(ctx, "openai_api_key"); value != "" {
		cfg.OpenAIAPIKey = value
	}
	if value := resolver.Get(ctx, "anthropic_api_key"); value != "" {
		cfg.AnthropicAPIKey = value
	}
	if value := resolver.Get(ctx, "google_api_key"); value != "" {
		cfg.GoogleAPIKey = value
	}
	if value := resolver.Get(ctx, "xai_api_key"); value != "" {
		cfg.XAIAPIKey = value
	}
	if value := resolver.Get(ctx, "azure_ai_api_key"); value != "" {
		cfg.AzureAIAPIKey = value
	}
	if value := resolver.Get(ctx, "huggingface_api_key"); value != "" {
		cfg.HuggingFaceAPIKey = value
	}
}

func CodexAPIKey() string {
	if value := strings.TrimSpace(Lookup("codex_api_key")); value != "" {
		return value
	}
	return strings.TrimSpace(Lookup("openai_api_key"))
}
