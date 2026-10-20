package api

import (
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"net/http"
)

func (s *Server) cancelUserIntent(w http.ResponseWriter, r *http.Request, channel model.ChannelID) {
	var request sessionintent.Request
	if err := decodeExactChannelJSON(w, r, "user message interruption", 128<<10, &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if request.State.ChannelID != channel {
		writeError(w, 400, "message interruption belongs to another channel")
		return
	}
	if err := s.requireClientWorkspaceIdentity(r.Context(), request.State.WorkspaceRoot, request.State.WorkspaceIdentity); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	receipt, err := s.repo.CancelUserIntent(r.Context(), request)
	if err != nil {
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, receipt)
}
