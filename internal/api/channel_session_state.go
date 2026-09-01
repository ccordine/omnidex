package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func (s *Server) getChannelSessionState(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	if err := validateExactQuery(r, "workspace_identity"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspaceIdentity, err := requiredWorkspaceIdentityQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := s.repo.ChannelSessionState(r.Context(), channelID, workspaceIdentity)
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
	if err := s.requireServerWorkspaceIdentity(
		state.WorkspaceRoot,
		workspaceIdentity,
	); err != nil {
		writeError(w, http.StatusConflict, "session workspace authority: "+err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, state)
}
