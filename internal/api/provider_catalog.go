package api

import (
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

type providerCatalogResponse struct {
	GenerationProvider string                `json:"generation_provider"`
	EmbeddingProvider  string                `json:"embedding_provider"`
	Providers          []providerCatalogItem `json:"providers"`
}

type providerCatalogItem struct {
	ID                    string   `json:"id"`
	DisplayName           string   `json:"display_name"`
	Aliases               []string `json:"aliases,omitempty"`
	Protocol              string   `json:"protocol"`
	DefaultBaseURL        string   `json:"default_base_url,omitempty"`
	ChineseService        bool     `json:"chinese_service"`
	SupportsGeneration    bool     `json:"supports_generation"`
	SupportsEmbeddings    bool     `json:"supports_embeddings"`
	SelectedForGeneration bool     `json:"selected_for_generation"`
	SelectedForEmbeddings bool     `json:"selected_for_embeddings"`
	GenerationModel       string   `json:"generation_model,omitempty"`
	EmbeddingModel        string   `json:"embedding_model,omitempty"`
	GenerationConfigured  bool     `json:"generation_configured"`
	EmbeddingConfigured   bool     `json:"embedding_configured"`
	ConfigurationError    string   `json:"configuration_error,omitempty"`
}

func (s *Server) handleProviderCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.providerCatalog())
}

func (s *Server) providerCatalog() providerCatalogResponse {
	cfg := s.providerConfiguration()
	response := providerCatalogResponse{
		GenerationProvider: canonicalProviderID(cfg.LLMProvider),
		EmbeddingProvider:  canonicalProviderID(cfg.EmbeddingProvider),
		Providers:          make([]providerCatalogItem, 0, len(catalog.Definitions())),
	}
	for _, definition := range catalog.Definitions() {
		generation := configuredProviderRoleStatus(cfg, definition.ID, configuredProviderModel(cfg, definition.ID, false), false)
		embedding := configuredProviderRoleStatus(cfg, definition.ID, configuredProviderModel(cfg, definition.ID, true), true)
		item := providerCatalogItem{
			ID:                    definition.ID,
			DisplayName:           definition.DisplayName,
			Aliases:               append([]string(nil), definition.Aliases...),
			Protocol:              string(definition.Protocol),
			DefaultBaseURL:        definition.DefaultBaseURL,
			ChineseService:        definition.ChineseService,
			SupportsGeneration:    definition.SupportsGeneration,
			SupportsEmbeddings:    definition.SupportsEmbeddings,
			SelectedForGeneration: response.GenerationProvider == definition.ID,
			SelectedForEmbeddings: response.EmbeddingProvider == definition.ID,
			GenerationModel:       generation.Model,
			EmbeddingModel:        embedding.Model,
			GenerationConfigured:  generation.Configured,
			EmbeddingConfigured:   embedding.Configured,
		}
		errors := make([]string, 0, 2)
		if item.SelectedForGeneration && generation.Error != "" {
			errors = append(errors, "generation: "+generation.Error)
		}
		if item.SelectedForEmbeddings && embedding.Error != "" {
			errors = append(errors, "embedding: "+embedding.Error)
		}
		item.ConfigurationError = strings.Join(errors, "; ")
		response.Providers = append(response.Providers, item)
	}
	return response
}

func canonicalProviderID(provider string) string {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return strings.ToLower(strings.TrimSpace(provider))
	}
	return definition.ID
}
