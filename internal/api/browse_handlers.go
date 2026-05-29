package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	if client := s.hostBridgeClient(); client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		result, err := client.Browse(ctx, target)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":    result.Path,
			"parent":  result.Parent,
			"entries": result.Entries,
			"source":  "host-bridge",
		})
		return
	}

	opts := hostbridge.BrowseOptions{}
	if s.repo != nil {
		projects, err := s.repo.ListProjects(r.Context(), 500, 0)
		if err == nil {
			for _, project := range projects {
				root := filepath.Clean(strings.TrimSpace(project.Location))
				if root != "" {
					opts.ExtraRoots = append(opts.ExtraRoots, root)
				}
			}
		}
	}
	result, err := hostbridge.ListDirectory(target, opts)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "outside allowed browse roots") {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    result.Path,
		"parent":  result.Parent,
		"entries": result.Entries,
		"source":  "core-local",
	})
}
