package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

type CompatibleProviderConfig struct {
	BaseURL      string
	APIKey       string
	Organization string
	Project      string
}

type ProviderModelConfig struct {
	Embedding string
}

func loadProviderSelection() (string, string) {
	return canonicalProviderSelection(os.Getenv("LLM_PROVIDER")),
		canonicalProviderSelection(os.Getenv("EMBEDDING_PROVIDER"))
}

func canonicalProviderSelection(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if definition, ok := catalog.Lookup(value); ok {
		return definition.ID
	}
	return value
}

func loadCompatibleProviderConfigs() map[string]CompatibleProviderConfig {
	providers := make(map[string]CompatibleProviderConfig)
	for _, definition := range catalog.ProductionDefinitions() {
		if definition.Protocol != catalog.ProtocolOpenAICompatible {
			continue
		}
		provider := CompatibleProviderConfig{
			BaseURL: firstNonEmptyEnv(definition.BaseURLEnvironmentKeys, definition.DefaultBaseURL),
			APIKey:  firstNonEmptyEnv(definition.APIKeyEnvironmentKeys, ""),
		}
		if definition.ID == "openai" {
			provider.Organization = strings.TrimSpace(os.Getenv("OPENAI_ORGANIZATION"))
			provider.Project = strings.TrimSpace(os.Getenv("OPENAI_PROJECT"))
		}
		providers[definition.ID] = provider
	}
	return providers
}

func loadProviderModelConfigs() map[string]ProviderModelConfig {
	models := make(map[string]ProviderModelConfig)
	for _, definition := range catalog.ProductionDefinitions() {
		models[definition.ID] = ProviderModelConfig{
			Embedding: firstNonEmptyEnv(definition.EnvironmentKeys("EMBEDDING_MODEL"), definition.DefaultEmbeddingModel),
		}
	}
	return models
}

func embeddingModelForProvider(provider string) string {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return ""
	}
	keys := append(definition.EnvironmentKeys("EMBEDDING_MODEL"), "EMBEDDING_MODEL")
	return firstNonEmptyEnv(keys, definition.DefaultEmbeddingModel)
}

func getenvProvider(provider, suffix, fallback string) string {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return fallback
	}
	return firstNonEmptyEnv(definition.EnvironmentKeys(suffix), fallback)
}

func firstNonEmptyEnv(keys []string, fallback string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(fallback)
}

func validateSelectedProvider(provider string, cfg Config, label string) error {
	if err := validateSelectedProviderEndpoint(provider, cfg, label); err != nil {
		return err
	}
	return validateSelectedProviderCredential(provider, cfg, label)
}

func validateSelectedProviderEndpoint(provider string, cfg Config, label string) error {
	if strings.TrimSpace(provider) == "" {
		return fmt.Errorf("%s is not configured", label)
	}
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return fmt.Errorf("%s contains unsupported provider %q", label, provider)
	}
	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return validateProviderBaseURL(definition, cfg.OllamaBaseURL, label)
	case catalog.ProtocolOpenAICompatible:
		providerConfig, configured := cfg.CompatibleProviders[definition.ID]
		if !configured {
			return fmt.Errorf("compatible provider configuration is missing for %s", definition.ID)
		}
		return validateProviderBaseURL(definition, providerConfig.BaseURL, label)
	case catalog.ProtocolAzure:
		return validateProviderBaseURL(definition, cfg.AzureAIBaseURL, label)
	case catalog.ProtocolGoogle:
		return validateProviderBaseURL(definition, cfg.GoogleBaseURL, label)
	case catalog.ProtocolHuggingFace:
		return validateProviderBaseURL(definition, cfg.HuggingFaceBaseURL, label)
	default:
		return fmt.Errorf("provider %q uses unsupported protocol %q", definition.ID, definition.Protocol)
	}
}

func validateSelectedProviderCredential(provider string, cfg Config, label string) error {
	if strings.TrimSpace(provider) == "" {
		return fmt.Errorf("%s is not configured", label)
	}
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return fmt.Errorf("%s contains unsupported provider %q", label, provider)
	}
	var value string
	fallback := "API key"
	switch definition.Protocol {
	case catalog.ProtocolOllama:
		return nil
	case catalog.ProtocolOpenAICompatible:
		providerConfig, configured := cfg.CompatibleProviders[definition.ID]
		if !configured {
			return fmt.Errorf("compatible provider configuration is missing for %s", definition.ID)
		}
		value = providerConfig.APIKey
	case catalog.ProtocolAzure:
		value = cfg.AzureAIAPIKey
		fallback = "Azure API key"
	case catalog.ProtocolGoogle:
		value = cfg.GoogleAPIKey
		fallback = "Google API key"
	case catalog.ProtocolHuggingFace:
		value = cfg.HuggingFaceAPIKey
		fallback = "Hugging Face API key"
	default:
		return fmt.Errorf("provider %q uses unsupported protocol %q", definition.ID, definition.Protocol)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required when %s=%s", environmentChoice(definition.APIKeyEnvironmentKeys, fallback), label, definition.ID)
	}
	return nil
}

// ValidateProviderConfiguration validates the credentials and endpoint required
// to construct a client for one provider. Callers that support runtime provider
// selection use this instead of duplicating protocol-specific validation.
func ValidateProviderConfiguration(cfg Config, provider, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("provider validation label is required")
	}
	return validateSelectedProvider(provider, cfg, label)
}

func validateProviderBaseURL(definition catalog.Definition, rawURL, label string) error {
	name := environmentChoice(definition.BaseURLEnvironmentKeys, "base URL")
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return fmt.Errorf("%s is required when %s=%s", name, label, definition.ID)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL, received %q", name, value)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain credentials, query parameters, or fragments, received %q", name, value)
	}
	return nil
}

func environmentChoice(keys []string, fallback string) string {
	if len(keys) == 0 {
		return fallback
	}
	return strings.Join(keys, " or ")
}

func CloneCompatibleProviders(values map[string]CompatibleProviderConfig) map[string]CompatibleProviderConfig {
	if values == nil {
		return nil
	}
	cloned := make(map[string]CompatibleProviderConfig, len(values))
	for provider, value := range values {
		cloned[provider] = value
	}
	return cloned
}

func CloneProviderModels(values map[string]ProviderModelConfig) map[string]ProviderModelConfig {
	if values == nil {
		return nil
	}
	cloned := make(map[string]ProviderModelConfig, len(values))
	for provider, value := range values {
		cloned[provider] = value
	}
	return cloned
}
