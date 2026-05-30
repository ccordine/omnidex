package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectgit"
)

func (s *Server) handleProjectGit(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	location := strings.TrimSpace(project.Location)
	if location == "" {
		writeError(w, http.StatusBadRequest, "project location is not set")
		return
	}
	payload, err := s.loadProjectGitStatus(r.Context(), project, location)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) loadProjectGitStatus(ctx context.Context, project model.Project, location string) (map[string]any, error) {
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	if projectPathAccessibleLocally(location) {
		return projectgit.CollectStatus(runCtx, location, "core-local")
	}
	if client := s.hostBridgeClient(); client != nil {
		return s.loadProjectGitStatusViaBridge(runCtx, location)
	}
	return nil, fmt.Errorf("project directory is not accessible locally")
}
