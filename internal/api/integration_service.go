package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const maxIntegrationItemPathBytes = 256

func (s *Server) handleIntegrationDataSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleDataSourceCreate(w, r)
}

func (s *Server) handleIntegrationChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.createChannel(w, r)
}

func (s *Server) handleIntegrationChannelByID(w http.ResponseWriter, r *http.Request) {
	parts, err := exactIntegrationPathParts(r, "/v1/integrations/channels/", 2)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	channelID := model.ChannelID(parts[0])
	if err := channelID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parts) == 1 {
		s.getIntegrationChannel(w, r, channelID)
		return
	}
	if parts[1] != "messages" {
		writeError(w, http.StatusNotFound, "integration channel route not found")
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.postChannelMessage(w, r, channelID)
	case http.MethodGet:
		s.listIntegrationChannelMessages(w, r, channelID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getIntegrationChannel(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channel, err := s.repo.GetChannel(r.Context(), channelID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if channel.Scope != model.ChannelScopeUser {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": channel})
}

func (s *Server) listIntegrationChannelMessages(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	if err := validateExactQuery(r, "limit", "before_id"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", defaultChannelHistoryLimit, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	beforeID, err := exactOptionalPositiveInt64Query(r, "before_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repo.ListChannelMessages(r.Context(), channelID, limit, beforeID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channel_id": channelID, "messages": page.Messages,
		"next_before_id": page.NextBeforeID, "has_more": page.HasMore,
	})
}

func (s *Server) handleIntegrationJobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts, err := exactIntegrationPathParts(r, "/v1/integrations/jobs/", 1)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != parts[0] {
		writeError(w, http.StatusBadRequest, "job ID must be one canonical positive integer")
		return
	}
	s.writeCurrentJobDetails(w, r, id)
}

func exactIntegrationPathParts(r *http.Request, prefix string, maximum int) ([]string, error) {
	if r == nil || r.URL == nil || len(r.URL.Path) > maxIntegrationItemPathBytes ||
		r.URL.EscapedPath() != r.URL.Path || !strings.HasPrefix(r.URL.Path, prefix) {
		return nil, fmt.Errorf("integration path must be one exact canonical path")
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(rest, "/")
	if rest == "" || len(parts) > maximum {
		return nil, fmt.Errorf("integration route not found")
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("integration route not found")
		}
	}
	return parts, nil
}
