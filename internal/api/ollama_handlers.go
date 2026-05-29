package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
)

func (s *Server) ollamaClient() *ollama.Client {
	endpoint := firstNonEmpty(s.ollamaBaseURL, "http://127.0.0.1:11434")
	return ollama.New(endpoint, s.ollamaDefaultModel, "", 15*time.Minute)
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
	if err := s.ollamaClient().DeleteModel(ctx, name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": name})
}

func (s *Server) listOllamaModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	client := s.ollamaClient()
	models, err := client.ListModels(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	configured := s.envModelConfig().OllamaModelNames()
	configuredSet := map[string]struct{}{}
	for _, name := range configured {
		configuredSet[name] = struct{}{}
	}
	items := make([]map[string]any, 0, len(models))
	for _, model := range models {
		_, inUse := configuredSet[model.Name]
		items = append(items, map[string]any{
			"name":        model.Name,
			"size":        model.Size,
			"modified_at": model.ModifiedAt,
			"configured":  inUse,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint":          firstNonEmpty(s.ollamaBaseURL, "http://127.0.0.1:11434"),
		"models":            items,
		"configured_models": configured,
	})
}

func (s *Server) pullOllamaModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	if err := s.ollamaClient().PullModel(ctx, model); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   model,
		"message": "model pulled",
	})
}
