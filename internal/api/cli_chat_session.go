package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const maxCLIChatSessionBootstrapBodyBytes int64 = 16 * 1024

type cliChatSessionBootstrapRequest struct {
	WorkspaceRoot     string `json:"workspace_root"`
	WorkspaceIdentity string `json:"workspace_identity"`
}

type cliChatSessionBootstrapResponse struct {
	WorkspaceIdentity string        `json:"workspace_identity"`
	Channel           model.Channel `json:"channel"`
}

func (s *Server) handleCLIChatSessionBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var request cliChatSessionBootstrapRequest
	if err := decodeExactChannelJSON(
		w,
		r,
		"CLI chat session bootstrap request",
		maxCLIChatSessionBootstrapBodyBytes,
		&request,
	); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := model.ValidateChannelWorkspaceRoot(request.WorkspaceRoot); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(
		request.WorkspaceRoot,
		request.WorkspaceIdentity,
	); err != nil {
		writeError(w, http.StatusConflict, "CLI session workspace authority: "+err.Error())
		return
	}
	channel, err := s.repo.EnsureCLIChatSessionChannel(
		r.Context(),
		request.WorkspaceRoot,
		request.WorkspaceIdentity,
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrCLIChatSessionConflict) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, cliChatSessionBootstrapResponse{
		WorkspaceIdentity: request.WorkspaceIdentity,
		Channel:           channel,
	})
}
