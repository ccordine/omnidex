package api

import (
	"context"
	"fmt"
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
	stored, err := s.rawStoredSecrets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"storage":  "database",
		"fields":   secrets.FieldList(stored),
		"resolved": secretsSnapshot(stored),
	})
}

func (s *Server) rawStoredSecrets(ctx context.Context) (map[string]string, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("API secret database is unavailable")
	}
	return s.repo.GetAPISecrets(ctx)
}

func (s *Server) handleAPISecretsPut(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	req, err := decodeAPISecretsRequest(w, r)
	if err != nil {
		writeError(w, exactSettingsErrorStatus(err), err.Error())
		return
	}
	stored, err := s.repo.SetAPISecrets(r.Context(), req.Values, req.ClearKeys)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"storage":          "database",
		"fields":           secrets.FieldList(stored),
		"resolved":         secretsSnapshot(stored),
		"restart_required": true,
		"message":          "API keys were stored. Restart the core service to activate provider credential changes consistently across API and workers.",
	})
}

func secretsSnapshot(stored map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, field := range secrets.Fields {
		out[field.Key] = strings.TrimSpace(stored[field.Key]) != ""
	}
	return out
}
