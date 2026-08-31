package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

const defaultChannelSessionMessageLimit = 100

type channelSessionResponse struct {
	Channel      model.Channel          `json:"channel"`
	Messages     []model.ChannelMessage `json:"messages"`
	NextBeforeID *int64                 `json:"next_before_id,omitempty"`
	HasMore      bool                   `json:"has_more"`
	ActiveJob    *model.JobDetails      `json:"active_job"`
}

func (s *Server) getChannelSession(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	if err := validateExactQuery(r, "limit"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := exactChannelQueryInteger(
		r,
		"limit",
		defaultChannelSessionMessageLimit,
		1,
		queue.MaxChannelSessionMessages,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot, err := s.repo.ChannelSessionSnapshot(r.Context(), channelID, limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload := channelSessionResponse{
		Channel:      snapshot.Channel,
		Messages:     snapshot.Transcript.Messages,
		NextBeforeID: snapshot.Transcript.NextBeforeID,
		HasMore:      snapshot.Transcript.HasMore,
		ActiveJob:    snapshot.ActiveJob,
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
