package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) handlePublicDataSources(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	request, err := dataSourcePageRequest(r, dataSourceAPIPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repo.ListDataSourcesPage(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sources": dataSourcesPublicList(page.Items), "offset": page.Offset, "has_more": page.HasMore,
		"next_offset": dataSourceNextOffset(page.Offset, len(page.Items), page.HasMore),
	})
}

func (s *Server) handlePublicDataSourceByID(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/data-sources/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusNotFound, "data source not found")
		return
	}
	sourceID := parts[0]
	if len(parts) == 2 {
		switch parts[1] {
		case "catalog":
			s.handleDataSourceCatalog(w, r, sourceID)
			return
		case "explore":
			s.handleDataSourceExplore(w, r, sourceID)
			return
		case "ask":
			s.handlePublicDataSourceAsk(w, r, sourceID)
			return
		}
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		record, err := s.repo.GetDataSource(r.Context(), sourceID)
		if err != nil {
			writeDataSourceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"source": dataSourcePublic(record)})
		return
	}
	if parts[1] != "channels" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if len(parts) == 2 {
		s.handleDataSourceChannels(w, r, sourceID)
		return
	}
	channelID := parts[2]
	if len(parts) == 3 {
		s.handleDataSourceChannelByID(w, r, sourceID, channelID)
		return
	}
	if len(parts) == 4 && parts[3] == "messages" {
		s.handleDataSourceChannelMessages(w, r, sourceID, channelID)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleDataSourceChannels(w http.ResponseWriter, r *http.Request, sourceID string) {
	if _, err := s.repo.GetDataSource(r.Context(), sourceID); err != nil {
		writeDataSourceError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		request, err := dataSourcePageRequest(r, dataSourceAPIPageSize)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		page, err := s.repo.ListDataSourceChannelsPage(r.Context(), sourceID, request)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channels": page.Items, "offset": page.Offset, "has_more": page.HasMore,
			"next_offset": dataSourceNextOffset(page.Offset, len(page.Items), page.HasMore),
		})
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		channel, err := s.repo.CreateDataSourceChannel(r.Context(), sourceID, req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"channel": channel})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataSourceChannelByID(w http.ResponseWriter, r *http.Request, sourceID, channelID string) {
	switch r.Method {
	case http.MethodGet:
		channel, err := s.repo.GetDataSourceChannel(r.Context(), sourceID, channelID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "channel not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channel": channel})
	case http.MethodDelete:
		if err := s.repo.DeleteDataSourceChannel(r.Context(), sourceID, channelID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "channel not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataSourceChannelMessages(w http.ResponseWriter, r *http.Request, sourceID, channelID string) {
	if _, err := s.repo.GetDataSourceChannel(r.Context(), sourceID, channelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		request, err := dataSourcePageRequest(r, dataSourceAPIPageSize)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		page, err := s.repo.ListDataSourceChannelMessagePage(r.Context(), channelID, request)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"messages": page.Items, "offset": page.Offset, "has_more": page.HasMore,
			"next_offset": dataSourceNextOffset(page.Offset, len(page.Items), page.HasMore),
		})
	case http.MethodPost:
		s.postDataSourceChannelMessage(w, r, sourceID, channelID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) postDataSourceChannelMessage(w http.ResponseWriter, r *http.Request, sourceID, channelID string) {
	writeRemovedInferenceAction(w, "data-source channel inference")
}

func (s *Server) handlePublicDataSourceAsk(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeRemovedInferenceAction(w, "public data-source natural-language query")
}
