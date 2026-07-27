package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/ollama"
)

type researchStatusResponse struct {
	GenerationProvider generationProviderStatus `json:"generation_provider"`
	EmbeddingProvider  generationProviderStatus `json:"embedding_provider"`
	Ollama             *ollamaRuntimeStatus     `json:"ollama,omitempty"`
	WebSearch          webSearchRuntimeStatus   `json:"web_search"`
	ResearchRunnable   bool                     `json:"research_runnable"`
	Warnings           []string                 `json:"warnings,omitempty"`
	HTML               string                   `json:"html"`
}

type generationProviderStatus struct {
	Provider   string `json:"provider"`
	Model      string `json:"model,omitempty"`
	State      string `json:"state"`
	Configured bool   `json:"configured"`
	Probed     bool   `json:"probed"`
	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
}

type ollamaRuntimeStatus struct {
	BaseURL             string   `json:"base_url,omitempty"`
	Reachable           bool     `json:"reachable"`
	ConfiguredModels    []string `json:"configured_models,omitempty"`
	AvailableModels     []string `json:"available_models,omitempty"`
	MissingModels       []string `json:"missing_models,omitempty"`
	EmbeddingModel      string   `json:"embedding_model,omitempty"`
	EmbeddingAvailable  bool     `json:"embedding_available"`
	LastProviderError   string   `json:"last_provider_error,omitempty"`
	RecommendedHostHint string   `json:"recommended_host_hint,omitempty"`
}

func (s *Server) collectResearchStatus(ctx context.Context) researchStatusResponse {
	cfg := s.providerConfiguration()
	generation := configuredProviderRoleStatus(cfg, cfg.LLMProvider, configuredProviderModel(cfg, cfg.LLMProvider, false), false)
	embedding := configuredProviderRoleStatus(cfg, cfg.EmbeddingProvider, configuredProviderModel(cfg, cfg.EmbeddingProvider, true), true)
	status := researchStatusResponse{
		GenerationProvider: generation,
		EmbeddingProvider:  embedding,
		WebSearch:          s.collectWebSearchStatus(ctx),
		ResearchRunnable:   generation.Configured && embedding.Configured,
	}
	if generation.Error != "" {
		status.Warnings = append(status.Warnings, "generation provider is misconfigured: "+generation.Error)
	}
	if embedding.Error != "" {
		status.Warnings = append(status.Warnings, "embedding provider is misconfigured: "+embedding.Error)
	}

	if generation.Provider == "ollama" || embedding.Provider == "ollama" {
		if err := config.ValidateProviderConfiguration(cfg, "ollama", "Ollama runtime"); err != nil {
			status.ResearchRunnable = false
			status.Warnings = append(status.Warnings, "Ollama runtime is misconfigured: "+err.Error())
		} else {
			runtime := s.collectOllamaRuntimeStatus(ctx, cfg)
			status.Ollama = &runtime
			if generation.Provider == "ollama" {
				applyOllamaRoleStatus(&status.GenerationProvider, runtime)
			}
			if embedding.Provider == "ollama" {
				applyOllamaRoleStatus(&status.EmbeddingProvider, runtime)
			}
			if !runtime.Reachable {
				status.ResearchRunnable = false
				status.Warnings = append(status.Warnings, "Ollama is unreachable from the core process; queued jobs will fail until OLLAMA_BASE_URL is reachable")
			}
			if len(runtime.MissingModels) > 0 {
				status.ResearchRunnable = false
				status.Warnings = append(status.Warnings, "one or more required Ollama models are missing")
			}
		}
	}
	if status.WebSearch.Enabled && !status.WebSearch.ReachableProvider {
		status.Warnings = append(status.Warnings, "no configured web search provider passed the reachability probe; research may run in degraded mode from local, docs, and memory context only")
	}
	return status
}

func configuredProviderRoleStatus(cfg config.Config, provider, model string, embedding bool) generationProviderStatus {
	role := "generation"
	if embedding {
		role = "embedding"
	}
	status := generationProviderStatus{
		Provider: strings.ToLower(strings.TrimSpace(provider)),
		Model:    strings.TrimSpace(model),
		State:    "misconfigured",
	}
	definition, ok := catalog.Lookup(status.Provider)
	if !ok {
		status.Error = fmt.Sprintf("unsupported %s provider %q", role, status.Provider)
		return status
	}
	status.Provider = definition.ID
	if (!embedding && !definition.SupportsGeneration) || (embedding && !definition.SupportsEmbeddings) {
		status.Error = fmt.Sprintf("provider %q does not support %s", definition.ID, role)
		return status
	}
	if status.Model == "" {
		status.Error = fmt.Sprintf("%s model is required for provider %q", role, definition.ID)
		return status
	}
	if err := config.ValidateProviderConfiguration(cfg, definition.ID, strings.ToUpper(role)+" provider"); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Configured = true
	status.State = "configured"
	return status
}

func configuredProviderModel(cfg config.Config, provider string, embedding bool) string {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return ""
	}
	if embedding && definition.ID == cfg.EmbeddingProvider {
		if model := strings.TrimSpace(cfg.EmbeddingModel); model != "" {
			return model
		}
	}
	if !embedding && definition.ID == cfg.LLMProvider {
		if model := strings.TrimSpace(cfg.DefaultModel); model != "" {
			return model
		}
	}
	models := cfg.ProviderModels[definition.ID]
	if embedding {
		if model := strings.TrimSpace(models.Embedding); model != "" {
			return model
		}
		return strings.TrimSpace(definition.DefaultEmbeddingModel)
	}
	if model := strings.TrimSpace(models.Default); model != "" {
		return model
	}
	return strings.TrimSpace(definition.DefaultModel)
}

func applyOllamaRoleStatus(status *generationProviderStatus, runtime ollamaRuntimeStatus) {
	if status == nil || status.Provider != "ollama" || !status.Configured {
		return
	}
	status.Probed = true
	status.Reachable = runtime.Reachable
	if !runtime.Reachable {
		status.State = "unreachable"
		status.Error = runtime.LastProviderError
		return
	}
	if !ollama.ModelIsAvailable(runtime.AvailableModels, status.Model) {
		status.State = "degraded"
		status.Error = fmt.Sprintf("configured Ollama model %q is not available", status.Model)
		return
	}
	status.State = "reachable"
}

func (s *Server) collectOllamaRuntimeStatus(ctx context.Context, cfg config.Config) ollamaRuntimeStatus {
	status := ollamaRuntimeStatus{
		BaseURL:             s.ollamaEndpoint(),
		ConfiguredModels:    configuredOllamaModels(cfg),
		RecommendedHostHint: "If core runs in Docker, OLLAMA_BASE_URL must be reachable from the core container and Ollama must listen on an accessible interface.",
	}
	if definition, ok := catalog.Lookup(cfg.EmbeddingProvider); ok && definition.ID == "ollama" {
		status.EmbeddingModel = configuredProviderModel(cfg, "ollama", true)
	}
	models, err := s.probeOllamaTags(ctx)
	if err != nil {
		status.LastProviderError = err.Error()
		return status
	}
	status.Reachable = true
	status.AvailableModels = models
	for _, model := range status.ConfiguredModels {
		if !ollama.ModelIsAvailable(models, model) {
			status.MissingModels = append(status.MissingModels, model)
		}
	}
	if status.EmbeddingModel != "" {
		status.EmbeddingAvailable = ollama.ModelIsAvailable(models, status.EmbeddingModel)
	}
	return status
}

func configuredOllamaModels(cfg config.Config) []string {
	values := make([]string, 0, 2)
	if definition, ok := catalog.Lookup(cfg.LLMProvider); ok && definition.ID == "ollama" {
		values = append(values, configuredProviderModel(cfg, "ollama", false))
	}
	if definition, ok := catalog.Lookup(cfg.EmbeddingProvider); ok && definition.ID == "ollama" {
		values = append(values, configuredProviderModel(cfg, "ollama", true))
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		model := strings.TrimSpace(value)
		if model == "" {
			continue
		}
		if _, duplicate := seen[model]; duplicate {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}
