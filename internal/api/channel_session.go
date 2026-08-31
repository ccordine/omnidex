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
	RealtimeCursor    uint64                        `json:"realtime_cursor"`
	Revision          string                        `json:"revision"`
	WorkspaceIdentity string                        `json:"workspace_identity"`
	Channel           model.Channel                 `json:"channel"`
	Messages          []model.ChannelMessage        `json:"messages"`
	NextBeforeID      *int64                        `json:"next_before_id,omitempty"`
	HasMore           bool                          `json:"has_more"`
	Turns             []queue.ChannelSessionTurn    `json:"turns"`
	TurnsTruncated    bool                          `json:"turns_truncated"`
	Controls          []queue.ChannelSessionControl `json:"controls"`
	ControlsTruncated bool                          `json:"controls_truncated"`
	ActiveJob         *model.JobDetails             `json:"active_job"`
}

func (s *Server) getChannelSession(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	if err := validateExactQuery(r, "limit", "workspace_identity"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspaceIdentity, err := requiredWorkspaceIdentityQuery(r)
	if err != nil {
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
	channel, err := s.repo.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(
		channel.WorkspaceRoot,
		workspaceIdentity,
	); err != nil {
		writeError(w, http.StatusConflict, "session workspace authority: "+err.Error())
		return
	}
	hub, err := s.requireRealtimeHub()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	realtimeCursor, err := hub.Cursor()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	snapshot, err := s.repo.ChannelSessionSnapshot(
		r.Context(),
		channelID,
		limit,
		workspaceIdentity,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if errors.Is(err, queue.ErrChannelSessionWorkspace) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload := channelSessionResponse{
		RealtimeCursor:    realtimeCursor,
		Revision:          snapshot.Revision,
		WorkspaceIdentity: snapshot.WorkspaceIdentity,
		Channel:           snapshot.Channel,
		Messages:          snapshot.Transcript.Messages,
		NextBeforeID:      snapshot.Transcript.NextBeforeID,
		HasMore:           snapshot.Transcript.HasMore,
		Turns:             snapshot.Turns,
		TurnsTruncated:    snapshot.TurnsTruncated,
		Controls:          snapshot.Controls,
		ControlsTruncated: snapshot.ControlsTruncated,
		ActiveJob:         snapshot.ActiveJob,
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
