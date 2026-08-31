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
	return os.Getenv("LLM_PROVIDER"), os.Getenv("EMBEDDING_PROVIDER")
}

func loadCompatibleProviderConfigs() map[string]CompatibleProviderConfig {
	providers := make(map[string]CompatibleProviderConfig)
	for _, definition := range catalog.ProductionDefinitions() {
		if definition.Protocol != catalog.ProtocolOpenAICompatible {
			continue
		}
		provider := CompatibleProviderConfig{
			BaseURL: getenv(definition.EnvironmentKey("BASE_URL"), definition.DefaultBaseURL),
			APIKey:  os.Getenv(definition.EnvironmentKey("API_KEY")),
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
			Embedding: getenv(definition.EnvironmentKey("EMBEDDING_MODEL"), definition.DefaultEmbeddingModel),
		}
	}
	return models
}

func embeddingModelForProvider(provider string, models map[string]ProviderModelConfig) string {
	definition, err := catalog.Resolve(provider)
	if err != nil {
		return ""
	}
	return models[definition.ID].Embedding
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
	definition, err := catalog.Resolve(provider)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", label, err)
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
	definition, err := catalog.Resolve(provider)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", label, err)
	}
	var value string
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
	case catalog.ProtocolGoogle:
		value = cfg.GoogleAPIKey
	case catalog.ProtocolHuggingFace:
		value = cfg.HuggingFaceAPIKey
	default:
		return fmt.Errorf("provider %q uses unsupported protocol %q", definition.ID, definition.Protocol)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required when %s=%s", definition.EnvironmentKey("API_KEY"), label, definition.ID)
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
	name := definition.EnvironmentKey("BASE_URL")
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
