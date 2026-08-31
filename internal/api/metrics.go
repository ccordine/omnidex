package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) handleMetricsLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	summary, err := s.repo.TelemetryLive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleMetricsRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateExactQuery(r, "limit"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 50, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	runs, err := s.repo.ListTelemetryRuns(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleMetricsRunByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/metrics/runs/")
	id = strings.Trim(id, "/")
	run, events, err := s.repo.GetTelemetryRun(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == pgx.ErrNoRows {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "events": events})
}

func (s *Server) handleMetricsModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.repo.TelemetryModelSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": items})
}

func (s *Server) handleMetricsContextShrink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateExactQuery(r, "limit", "source"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 100, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, err := exactMetricsSource(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository mode required for context shrink metrics")
		return
	}
	metrics, err := s.repo.ContextShrinkMetrics(r.Context(), source, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleMetricsContextUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateExactQuery(r, "limit", "source"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 100, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, err := exactMetricsSource(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository mode required for context usage metrics")
		return
	}
	metrics, err := s.repo.LLMContextUsageMetrics(r.Context(), source, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleMetricsOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository mode required for operations metrics")
		return
	}
	metrics, err := s.repo.OperationsMetrics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func exactMetricsSource(r *http.Request) (string, error) {
	values, exists := r.URL.Query()["source"]
	if !exists {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" || values[0] != strings.TrimSpace(values[0]) {
		return "", fmt.Errorf("source must be one non-empty canonical string")
	}
	return values[0], nil
}
