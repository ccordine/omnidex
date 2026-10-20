package api

import (
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"net/http"
)

func (s *Server) handleCLISessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r, "workspace_root", "workspace_identity", "after"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	root, identity := r.URL.Query().Get("workspace_root"), r.URL.Query().Get("workspace_identity")
	clientID, _, err := projectroot.ParseClientWorkspaceIdentity(identity)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireClientWorkspaceIdentity(r.Context(), root, identity); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	after := model.ChannelID(r.URL.Query().Get("after"))
	if after != "" && !projectroot.IsCLIChatChannelID(after) {
		writeError(w, http.StatusBadRequest, "invalid sessions cursor")
		return
	}
	listing, err := s.repo.CLISessions(r.Context(), clientID, after)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range listing.Sessions {
		entry := &listing.Sessions[i]
		entry.Connected = s.workspaceConnections.Connected(entry.Directory, entry.WorkspaceIdentity)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, listing)
}
