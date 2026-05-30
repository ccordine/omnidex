package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
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
	items, err := s.repo.ListDataSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": dataSourcesPublicList(items)})
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
		channels, err := s.repo.ListDataSourceChannels(r.Context(), sourceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
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
		limit := parseInt(r.URL.Query().Get("limit"), 80)
		messages, err := s.repo.ListDataSourceChannelMessages(r.Context(), channelID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
	case http.MethodPost:
		s.postDataSourceChannelMessage(w, r, sourceID, channelID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) postDataSourceChannelMessage(w http.ResponseWriter, r *http.Request, sourceID, channelID string) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), sourceID)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	userMessage, err := s.repo.AddDataSourceChannelMessage(r.Context(), channelID, "user", prompt, json.RawMessage(`{}`), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	metadata, err := datasource.JobMetadata(record.ID, record.Name, prompt, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := s.repo.EnqueueJob(r.Context(), prompt, model.PipelineDataQuery, metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"user_message": userMessage,
		"job":          job,
		"message":      fmt.Sprintf("Queued data query job #%d", job.ID),
	})
}

func (s *Server) handlePublicDataSourceAsk(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	metadata, err := datasource.JobMetadata(record.ID, record.Name, question, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := s.repo.EnqueueJob(r.Context(), question, model.PipelineDataQuery, metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":      job,
		"question": question,
		"message":  fmt.Sprintf("Queued data query job #%d", job.ID),
	})
}
