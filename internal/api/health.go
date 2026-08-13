package api

import (
	"net/http"
	"strings"
	"time"
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
		"dependencies":    dependencies,
	})
}
