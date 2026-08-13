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

func loadProviderSelection() (string, string, error) {
	generationRaw := strings.TrimSpace(os.Getenv("LLM_PROVIDER"))
	if generationRaw == "" {
		generationRaw = "ollama"
	}
	generation, ok := catalog.Lookup(generationRaw)
	if !ok || !generation.SupportsExactPreparedStations {
		return "", "", fmt.Errorf(
			"LLM_PROVIDER must implement the exact prepared station contract; supported providers: %s",
			strings.Join(catalog.ExactStationProviderIDs(), ", "),
		)
	}

	embeddingRaw := strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER"))
	if embeddingRaw == "" {
		if !generation.SupportsEmbeddings {
			return "", "", fmt.Errorf("EMBEDDING_PROVIDER is required when LLM_PROVIDER=%s because that provider does not expose a supported embeddings API", generation.ID)
		}
		embeddingRaw = generation.ID
	}
	embedding, ok := catalog.Lookup(embeddingRaw)
	if !ok || !embedding.SupportsEmbeddings {
		return "", "", fmt.Errorf("EMBEDDING_PROVIDER must be one of: %s", strings.Join(catalog.EmbeddingProviderIDs(), ", "))
	}
	return generation.ID, embedding.ID, nil
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

func validateSelectedProviderModels(cfg Config) error {
	embedding, ok := catalog.Lookup(cfg.EmbeddingProvider)
	if !ok {
		return fmt.Errorf("EMBEDDING_PROVIDER contains unsupported provider %q", cfg.EmbeddingProvider)
	}
	if strings.TrimSpace(cfg.EmbeddingModel) == "" {
		return fmt.Errorf("%s is required when EMBEDDING_PROVIDER=%s", modelEnvironmentName(embedding, true), embedding.ID)
	}
	return nil
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

func modelEnvironmentName(definition catalog.Definition, embedding bool) string {
	if !embedding {
		return "exact station model"
	}
	keys := definition.EnvironmentKeys("EMBEDDING_MODEL")
	if len(keys) == 0 {
		return "EMBEDDING_MODEL"
	}
	return keys[0]
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
