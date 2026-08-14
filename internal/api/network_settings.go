package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/network"
)

func (s *Server) resolveCoreURL(r *http.Request) (string, string, error) {
	if s.repo != nil {
		stored, err := s.repo.GetCoreURL(r.Context())
		if err != nil {
			return "", "", fmt.Errorf("load core URL from PostgreSQL: %w", err)
		}
		if strings.TrimSpace(stored) != "" {
			return network.NormalizeCoreURL(stored), "database", nil
		}
	}
	if strings.TrimSpace(s.coreURLDefault) != "" {
		return network.NormalizeCoreURL(s.coreURLDefault), "environment", nil
	}
	return network.DefaultCoreURL(), "default", nil
}

func (s *Server) networkSettingsPayload(r *http.Request) (map[string]any, error) {
	coreURL, source, err := s.resolveCoreURL(r)
	if err != nil {
		return nil, err
	}
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
		"core_url":    coreURL,
		"source":      source,
		"host":        host,
		"port":        port,
		"listen_addr": strings.TrimSpace(s.listenAddr),
		"request_url": requestURL,
		"default_url": network.DefaultCoreURL(),
	}, nil
}

func (s *Server) handleNetworkSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload, err := s.networkSettingsPayload(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		if s.repo == nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		req, err := decodeNetworkSettingsRequest(w, r)
		if err != nil {
			writeError(w, exactSettingsErrorStatus(err), err.Error())
			return
		}
		coreURL := network.BuildCoreURL(req.Host, req.Port)
		stored, err := s.repo.SetCoreURL(r.Context(), coreURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payload, err := s.networkSettingsPayload(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("core URL %q was saved but could not be reloaded: %v", stored, err))
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
