package api

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func (s *Server) configuredDefaultLLMModel() string {
	if s == nil {
		return ""
	}
	cfg := s.providerConfiguration()
	if modelName := strings.TrimSpace(cfg.DefaultModel); modelName != "" {
		return modelName
	}
	provider := strings.TrimSpace(s.defaultProvider)
	return strings.TrimSpace(cfg.ProviderModels[provider].Default)
}

func (s *Server) requiredDefaultLLMModel() (string, error) {
	if s == nil {
		return "", fmt.Errorf("LLM model routing requires a server")
	}
	provider := strings.TrimSpace(s.defaultProvider)
	definition, ok := catalog.Lookup(provider)
	if !ok || !definition.SupportsGeneration {
		return "", fmt.Errorf("unsupported default LLM provider %q", provider)
	}
	provider = definition.ID
	modelName := s.configuredDefaultLLMModel()
	if modelName == "" {
		return "", fmt.Errorf("default model is required for provider %q", provider)
	}
	return modelName, nil
}
