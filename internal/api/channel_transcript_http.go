package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (s *Server) listChannelMessages(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	if err := validateExactQuery(r, "limit", "before_id", "required_message_id"); err != nil {
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
	requiredMessageID, err := exactOptionalPositiveInt64Query(r, "required_message_id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.channelStore.ListChannelMessages(r.Context(), channelID, limit, beforeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if requiredMessageID != nil && !containsRequiredUserMessage(page.Messages, *requiredMessageID) {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"channel transcript does not contain required message %d", *requiredMessageID,
		))
		return
	}
	payload, err := channelTranscriptResponseFor(channelID, page, beforeID != nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}

func exactOptionalPositiveInt64Query(r *http.Request, key string) (*int64, error) {
	values, exists := r.URL.Query()[key]
	if !exists {
		return nil, nil
	}
	if len(values) != 1 || values[0] == "" {
		return nil, fmt.Errorf("%s must be one positive canonical integer", key)
	}
	value, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || value < 1 || strconv.FormatInt(value, 10) != values[0] {
		return nil, fmt.Errorf("%s must be one positive canonical integer", key)
	}
	return &value, nil
}

func containsRequiredUserMessage(messages []model.ChannelMessage, requiredID int64) bool {
	for _, message := range messages {
		if message.ID == requiredID && message.Role == model.ChannelMessageRoleUser {
			return true
		}
	}
	return false
}
