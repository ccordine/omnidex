package api

import (
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

type providerCatalogResponse struct {
	ExactStationProvider string                `json:"exact_station_provider"`
	EmbeddingProvider    string                `json:"embedding_provider"`
	Providers            []providerCatalogItem `json:"providers"`
}

type providerCatalogItem struct {
	ID                    string   `json:"id"`
	DisplayName           string   `json:"display_name"`
	Protocol              string   `json:"protocol"`
	DefaultBaseURL        string   `json:"default_base_url,omitempty"`
	ChineseService        bool     `json:"chinese_service"`
	SupportsExactStations bool     `json:"supports_exact_stations"`
	SupportsEmbeddings    bool     `json:"supports_embeddings"`
	SelectedForStations   bool     `json:"selected_for_stations"`
	SelectedForEmbeddings bool     `json:"selected_for_embeddings"`
	EmbeddingModel        string   `json:"embedding_model,omitempty"`
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
		ExactStationProvider: cfg.LLMProvider,
		EmbeddingProvider:    cfg.EmbeddingProvider,
		Providers:            make([]providerCatalogItem, 0, len(catalog.ProductionDefinitions())),
	}
	for _, definition := range catalog.ProductionDefinitions() {
		embeddingModel := strings.TrimSpace(cfg.ProviderModels[definition.ID].Embedding)
		if response.EmbeddingProvider == definition.ID {
			embeddingModel = strings.TrimSpace(cfg.EmbeddingModel)
		}
		item := providerCatalogItem{
			ID: definition.ID, DisplayName: definition.DisplayName,
			Protocol: string(definition.Protocol),
			DefaultBaseURL: definition.DefaultBaseURL, ChineseService: definition.ChineseService,
			SupportsExactStations: definition.SupportsExactPreparedStations,
			SupportsEmbeddings:    definition.SupportsEmbeddings,
			SelectedForStations:   response.ExactStationProvider == definition.ID,
			SelectedForEmbeddings: response.EmbeddingProvider == definition.ID,
			EmbeddingModel:        embeddingModel,
		}
		response.Providers = append(response.Providers, item)
	}
	return response
}
