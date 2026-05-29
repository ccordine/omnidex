package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/network"
)

func (s *Server) resolveCoreURL(r *http.Request) (string, string) {
	if s.repo != nil {
		if stored, err := s.repo.GetCoreURL(r.Context()); err == nil && strings.TrimSpace(stored) != "" {
			return network.NormalizeCoreURL(stored), "database"
		}
	}
	if strings.TrimSpace(s.coreURLDefault) != "" {
		return network.NormalizeCoreURL(s.coreURLDefault), "environment"
	}
	return network.DefaultCoreURL(), "default"
}

func (s *Server) networkSettingsPayload(r *http.Request) map[string]any {
	coreURL, source := s.resolveCoreURL(r)
	host, port := network.ParseHostPort(coreURL)
	requestURL := ""
	if r != nil && strings.TrimSpace(r.Host) != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		requestURL = scheme + "://" + r.Host
	}
	return map[string]any{
		"core_url":      coreURL,
		"source":        source,
		"host":          host,
		"port":          port,
		"listen_addr":   strings.TrimSpace(s.listenAddr),
		"request_url":   requestURL,
		"default_url":   network.DefaultCoreURL(),
	}
}

func (s *Server) handleNetworkSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.networkSettingsPayload(r))
	case http.MethodPut:
		if s.repo == nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		var req struct {
			Host string `json:"host"`
			Port int    `json:"port"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		coreURL := strings.TrimSpace(req.URL)
		if coreURL == "" {
			coreURL = network.BuildCoreURL(req.Host, req.Port)
		}
		stored, err := s.repo.SetCoreURL(r.Context(), coreURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if path, err := resolveEnvFilePath(); err == nil {
			_ = writeEnvFile(path, map[string]string{"CORE_URL": stored})
		}
		writeJSON(w, http.StatusOK, s.networkSettingsPayload(r))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
