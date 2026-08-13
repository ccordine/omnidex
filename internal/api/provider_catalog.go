package api

import (
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

type providerCatalogResponse struct {
	ExactStationProvider string                `json:"exact_station_provider"`
	EmbeddingProvider    string                `json:"embedding_provider"`
	Providers            []providerCatalogItem `json:"providers"`
}

type providerCatalogItem struct {
	ID                      string   `json:"id"`
	DisplayName             string   `json:"display_name"`
	Aliases                 []string `json:"aliases,omitempty"`
	Protocol                string   `json:"protocol"`
	DefaultBaseURL          string   `json:"default_base_url,omitempty"`
	ChineseService          bool     `json:"chinese_service"`
	SupportsExactStations   bool     `json:"supports_exact_stations"`
	SupportsEmbeddings      bool     `json:"supports_embeddings"`
	SelectedForStations     bool     `json:"selected_for_stations"`
	SelectedForEmbeddings   bool     `json:"selected_for_embeddings"`
	EmbeddingModel          string   `json:"embedding_model,omitempty"`
	ExactStationsConfigured bool     `json:"exact_stations_configured"`
	EmbeddingConfigured     bool     `json:"embedding_configured"`
	ConfigurationError      string   `json:"configuration_error,omitempty"`
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
		ExactStationProvider: canonicalProviderID(cfg.LLMProvider),
		EmbeddingProvider:    canonicalProviderID(cfg.EmbeddingProvider),
		Providers:            make([]providerCatalogItem, 0, len(catalog.ProductionDefinitions())),
	}
	for _, definition := range catalog.ProductionDefinitions() {
		_, stationsConfigured, stationError := providerTransportStatus(cfg, definition.ID, false)
		embeddingModel, embeddingConfigured, embeddingError := providerTransportStatus(cfg, definition.ID, true)
		item := providerCatalogItem{
			ID:                      definition.ID,
			DisplayName:             definition.DisplayName,
			Aliases:                 append([]string(nil), definition.Aliases...),
			Protocol:                string(definition.Protocol),
			DefaultBaseURL:          definition.DefaultBaseURL,
			ChineseService:          definition.ChineseService,
			SupportsExactStations:   definition.SupportsExactPreparedStations,
			SupportsEmbeddings:      definition.SupportsEmbeddings,
			SelectedForStations:     response.ExactStationProvider == definition.ID,
			SelectedForEmbeddings:   response.EmbeddingProvider == definition.ID,
			EmbeddingModel:          embeddingModel,
			ExactStationsConfigured: stationsConfigured,
			EmbeddingConfigured:     embeddingConfigured,
		}
		errors := make([]string, 0, 2)
		if item.SelectedForStations && stationError != "" {
			errors = append(errors, "exact stations: "+stationError)
		}
		if item.SelectedForEmbeddings && embeddingError != "" {
			errors = append(errors, "embedding: "+embeddingError)
		}
		item.ConfigurationError = strings.Join(errors, "; ")
		response.Providers = append(response.Providers, item)
	}
	return response
}

func providerTransportStatus(cfg config.Config, provider string, embedding bool) (string, bool, string) {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return "", false, "unsupported provider"
	}
	selected := definition.ID == canonicalProviderID(cfg.LLMProvider)
	modelName := ""
	role := "exact station inference"
	if embedding {
		selected = definition.ID == canonicalProviderID(cfg.EmbeddingProvider)
		modelName = strings.TrimSpace(cfg.ProviderModels[definition.ID].Embedding)
		role = "embedding"
	}
	if selected {
		if embedding {
			modelName = strings.TrimSpace(cfg.EmbeddingModel)
		}
	}
	if (!embedding && !definition.SupportsExactPreparedStations) || (embedding && !definition.SupportsEmbeddings) {
		return modelName, false, "provider does not support " + role
	}
	if embedding && modelName == "" {
		return "", false, role + " model is not configured"
	}
	if !selected {
		return modelName, false, ""
	}
	if err := config.ValidateProviderConfiguration(cfg, definition.ID, role+" provider"); err != nil {
		return modelName, false, err.Error()
	}
	return modelName, true, ""
}

func canonicalProviderID(provider string) string {
	definition, ok := catalog.Lookup(provider)
	if !ok {
		return strings.ToLower(strings.TrimSpace(provider))
	}
	return definition.ID
}
