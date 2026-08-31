package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/version"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	coreURL, source, err := s.resolveCoreURL(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	dependencies := s.collectCoreDependencies(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          coreHealthStatus(dependencies),
		"time":            time.Now().UTC(),
		"queue_enabled":   s.repo != nil,
		"core_url":        coreURL,
		"core_url_source": source,
		"listen_addr":     strings.TrimSpace(s.listenAddr),
		"release":         version.JSON(),
		"dependencies":    dependencies,
	})
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	dependencies := s.collectCoreDependencies(r.Context())
	status := coreHealthStatus(dependencies)
	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{
		"status":       status,
		"time":         time.Now().UTC(),
		"dependencies": dependencies,
	})
}
