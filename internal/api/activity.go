package api

import (
	"net/http"
	"strings"
)

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	jobs, err := s.repo.ListJobs(r.Context(), "", minInt(limit, 30), 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := s.repo.ListTelemetryEvents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	memories, err := s.repo.ListMemoryChunks(r.Context(), "", nil, minInt(limit, 30))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":             jobs,
		"telemetry_events": events,
		"memories":         memories,
	})
}

func (s *Server) listMemory(w http.ResponseWriter, r *http.Request) {
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50)
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	tags := splitCommaQuery(r.URL.Query().Get("tags"))
	memories, err := s.repo.ListMemoryChunks(r.Context(), kind, tags, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": memories})
}

func splitCommaQuery(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
