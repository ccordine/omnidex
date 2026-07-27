package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func (s *Server) resolvePersonaLLM(req personaRequest) (resolvedPersonaLLM, error) {
	if s.llmClient == nil {
		return resolvedPersonaLLM{}, fmt.Errorf("llm client is not configured")
	}

	baseModel := strings.TrimSpace(req.Model)
	if req.LLM == nil {
		return resolvedPersonaLLM{Client: s.llmClient, Provider: s.defaultProvider, Model: baseModel}, nil
	}

	requestedProvider := normalizePersonaProvider(req.LLM.Provider)
	if requestedProvider == "" {
		requestedProvider = s.defaultProvider
	}
	definition, supported := catalog.Lookup(requestedProvider)
	if !supported || !definition.SupportsGeneration {
		return resolvedPersonaLLM{}, personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("llm.provider %q is unsupported (allowed: %s)", strings.TrimSpace(req.LLM.Provider), strings.Join(catalog.GenerationProviderIDs(), ", ")),
		}
	}
	requestedProvider = definition.ID
	if req.LLM.Compatible != nil && definition.Protocol != catalog.ProtocolOpenAICompatible {
		return resolvedPersonaLLM{}, personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("llm.compatible fields are only valid for OpenAI-compatible providers; %s uses protocol %s", definition.ID, definition.Protocol),
		}
	}

	requestedModel := firstNonEmpty(strings.TrimSpace(req.LLM.Model), baseModel)
	if requestedProvider == s.defaultProvider && !personaLLMRequiresDedicatedClient(req.LLM) {
		return resolvedPersonaLLM{Client: s.llmClient, Provider: requestedProvider, Model: requestedModel}, nil
	}

	cfg, resolvedModel, err := s.personaProviderConfig(requestedProvider, requestedModel, req.LLM.Compatible)
	if err != nil {
		return resolvedPersonaLLM{}, err
	}
	client, err := llmprovider.NewProvider(cfg, llmprovider.Options{
		Provider: requestedProvider,
		Model:    resolvedModel,
		Timeout:  s.requestTimeout,
	})
	if err != nil {
		return resolvedPersonaLLM{}, err
	}

	return resolvedPersonaLLM{Client: client, Provider: requestedProvider, Model: resolvedModel}, nil
}

func (s *Server) personaProviderConfig(provider, requestedModel string, compatible *personaCompatibleConfig) (config.Config, string, error) {
	definition, ok := catalog.Lookup(provider)
	if !ok || !definition.SupportsGeneration {
		return config.Config{}, "", personaRequestError{StatusCode: http.StatusBadRequest, Message: fmt.Sprintf("unsupported provider %q", provider)}
	}

	cfg := s.providerConfiguration()
	cfg.LLMProvider = definition.ID
	cfg.RequestTimeout = s.requestTimeout
	if definition.Protocol == catalog.ProtocolOllama {
		cfg.OllamaBaseURL = strings.TrimSpace(s.ollamaBaseURL)
	}

	model := strings.TrimSpace(requestedModel)
	if model == "" && definition.ID == s.defaultProvider {
		model = strings.TrimSpace(cfg.DefaultModel)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.ProviderModels[definition.ID].Default)
	}
	if model == "" {
		return cfg, "", personaRequestError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("%s provider requested but %s is not configured", definition.ID, providerModelEnvironmentName(definition)),
		}
	}
	cfg.DefaultModel = model
	if cfg.ProviderModels == nil {
		cfg.ProviderModels = make(map[string]config.ProviderModelConfig)
	}
	providerModels := cfg.ProviderModels[definition.ID]
	providerModels.Default = model
	cfg.ProviderModels[definition.ID] = providerModels

	if compatible != nil {
		if cfg.CompatibleProviders == nil {
			cfg.CompatibleProviders = make(map[string]config.CompatibleProviderConfig)
		}
		cfg.CompatibleProviders[definition.ID] = config.CompatibleProviderConfig{
			APIKey:       strings.TrimSpace(compatible.APIKey),
			BaseURL:      strings.TrimSpace(compatible.BaseURL),
			Organization: strings.TrimSpace(compatible.Organization),
			Project:      strings.TrimSpace(compatible.Project),
		}
	}
	if err := config.ValidateProviderConfiguration(cfg, definition.ID, "llm.provider"); err != nil {
		return cfg, model, personaRequestError{StatusCode: http.StatusBadRequest, Message: err.Error()}
	}
	return cfg, model, nil
}

func personaLLMRequiresDedicatedClient(req *personaLLMRequest) bool {
	if req == nil {
		return false
	}
	return normalizePersonaProvider(req.Provider) != "" || req.Compatible != nil
}

func normalizePersonaProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if definition, ok := catalog.Lookup(value); ok {
		return definition.ID
	}
	return value
}

func providerModelEnvironmentName(definition catalog.Definition) string {
	keys := definition.EnvironmentKeys("MODEL")
	if len(keys) == 0 {
		return "an explicit model"
	}
	return keys[0]
}
