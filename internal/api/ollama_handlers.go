package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) ollamaClient() *ollama.Client {
	return s.ollamaClientWithTimeout(15 * time.Minute)
}

func (s *Server) handleOllamaModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listOllamaModels(w, r)
	case http.MethodPost:
		s.pullOllamaModel(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOllamaModelByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/ollama/models/")
	name = strings.TrimSpace(strings.Trim(name, "/"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.requireOllamaModelUnused(ctx, name); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.ollamaLifecycleClient().DeleteModel(ctx, name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

func (s *Server) listOllamaModels(w http.ResponseWriter, r *http.Request) {
	limit, err := exactChannelQueryInteger(r, "limit", 50, 1, ollama.MaxModelPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	page, err := s.listInstalledOllamaModels(ctx, limit, offset)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	modelConfig := s.runtimeModelConfig()
	configured := []string{}
	if s.defaultProvider == "ollama" {
		configured = modelConfig.ModelNames()
	}
	providerConfig := s.providerConfiguration()
	if providerConfig.EmbeddingProvider == "ollama" {
		embeddingModel := strings.TrimSpace(providerConfig.EmbeddingModel)
		if embeddingModel != "" {
			configured = append(configured, embeddingModel)
		}
	}
	configuredSet := map[string]struct{}{}
	uniqueConfigured := make([]string, 0, len(configured))
	for _, name := range configured {
		if _, duplicate := configuredSet[name]; duplicate {
			continue
		}
		configuredSet[name] = struct{}{}
		uniqueConfigured = append(uniqueConfigured, name)
	}
	configured = uniqueConfigured
	items := make([]map[string]any, 0, len(page.Models))
	for _, model := range page.Models {
		_, inUse := configuredSet[model.Name]
		items = append(items, map[string]any{
			"name":           model.Name,
			"size":           model.Size,
			"modified_at":    model.ModifiedAt,
			"configured":     inUse,
			"family":         model.Details.Family,
			"parameter_size": model.Details.ParameterSize,
			"quantization":   model.Details.QuantizationLevel,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint":          s.ollamaEndpoint(),
		"models":            items,
		"configured_models": configured,
		"offset":            page.Offset,
		"has_more":          page.HasMore,
		"next_offset":       dataSourceNextOffset(page.Offset, len(page.Models), page.HasMore),
	})
}

func (s *Server) pullOllamaModel(w http.ResponseWriter, r *http.Request) {
	req, err := decodeOllamaPullRequest(w, r)
	if err != nil {
		writeError(w, exactSettingsErrorStatus(err), err.Error())
		return
	}
	if s.ollamaDownloads == nil {
		writeError(w, http.StatusServiceUnavailable, "Ollama downloads require PostgreSQL")
		return
	}
	download, err := s.ollamaDownloads.CreateOllamaModelDownload(r.Context(), req.Model)
	if err != nil {
		if errors.Is(err, queue.ErrOllamaModelDownloadActive) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": err.Error(), "download": download,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := download.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid durable Ollama download: "+err.Error())
		return
	}
	s.launchOllamaModelDownload(download)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"model": req.Model, "download": download,
	})
}
