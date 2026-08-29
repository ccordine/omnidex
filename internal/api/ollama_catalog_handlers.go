package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollamacatalog"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleOllamaCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query, err := exactUIQuery(r, "query", 128)
	if err != nil || strings.TrimSpace(query) == "" {
		if err == nil {
			err = fmt.Errorf("Ollama catalog query is required")
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pageNumber, err := exactChannelQueryInteger(r, "page", 1, 1, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	page, err := s.searchOllamaCatalog(ctx, query, pageNumber)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleOllamaDownloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.ollamaDownloads == nil {
		writeError(w, http.StatusServiceUnavailable, "Ollama downloads require PostgreSQL")
		return
	}
	limit, err := exactChannelQueryInteger(
		r, "limit", 20, 1, queue.MaxOllamaDownloadPageSize,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.ollamaDownloads.ListOllamaModelDownloads(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) searchOllamaCatalog(
	ctx context.Context,
	query string,
	page int,
) (ollamacatalog.Page, error) {
	catalog := s.ollamaCatalog
	if catalog == nil {
		catalog = ollamacatalog.NewOfficial(20 * time.Second)
	}
	return catalog.Search(ctx, query, page)
}
