package hostbridge

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/projectgit"
)

func (s *Server) handleProjectGit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	workspace, err := resolveHostWorkspace(rawPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	payload, err := projectgit.CollectStatus(ctx, workspace, "host-bridge")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if rawPath != "" && rawPath != workspace {
		payload.RequestedLocation = rawPath
	}
	if err := payload.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
