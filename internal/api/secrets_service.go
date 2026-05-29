package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/secrets"
)

func (s *Server) handleAPISecrets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAPISecretsGet(w, r)
	case http.MethodPut:
		s.handleAPISecretsPut(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPISecretsGet(w http.ResponseWriter, r *http.Request) {
	stored := s.rawStoredSecrets(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"storage":  "database",
		"fields":   secrets.FieldList(stored),
		"resolved": s.secretsSnapshot(r.Context()),
	})
}

func (s *Server) rawStoredSecrets(ctx context.Context) map[string]string {
	if s.repo != nil {
		if values, err := s.repo.GetAPISecrets(ctx); err == nil {
			return values
		}
	}
	if s.secretsResolver != nil {
		return s.secretsResolver.RawStored(ctx)
	}
	return map[string]string{}
}

func (s *Server) handleAPISecretsPut(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	var req struct {
		Values    map[string]string `json:"values"`
		ClearKeys []string          `json:"clear_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updates := map[string]string{}
	for key, value := range req.Values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			updates[key] = value
		}
	}
	stored, err := s.repo.SetAPISecrets(r.Context(), updates, req.ClearKeys)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.secretsResolver != nil {
		s.secretsResolver.Invalidate()
	}
	s.applyStoredSecrets(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"storage":  "database",
		"fields":   secrets.FieldList(stored),
		"resolved": s.secretsSnapshot(r.Context()),
	})
}

func (s *Server) secretsSnapshot(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	for _, field := range secrets.Fields {
		out[field.Key] = strings.TrimSpace(s.secretValue(ctx, field.Key)) != ""
	}
	return out
}

func (s *Server) secretValue(ctx context.Context, key string) string {
	if s.secretsResolver != nil {
		if value := strings.TrimSpace(s.secretsResolver.Get(ctx, key)); value != "" {
			return value
		}
	}
	for _, field := range secrets.Fields {
		if field.Key == key {
			return secrets.LookupEnv(field.EnvKeys)
		}
	}
	return ""
}

func (s *Server) applyStoredSecrets(ctx context.Context) {
	if value := s.secretValue(ctx, "openai_api_key"); value != "" {
		s.openAIAPIKey = value
	}
	if value := s.secretValue(ctx, "anthropic_api_key"); value != "" {
		s.anthropicAPIKey = value
	}
	if value := s.secretValue(ctx, "google_api_key"); value != "" {
		s.googleAPIKey = value
	}
	if value := s.secretValue(ctx, "xai_api_key"); value != "" {
		s.xAIAPIKey = value
	}
	if value := s.secretValue(ctx, "azure_ai_api_key"); value != "" {
		s.azureAIAPIKey = value
	}
	if value := s.secretValue(ctx, "huggingface_api_key"); value != "" {
		s.huggingFaceAPIKey = value
	}
}
