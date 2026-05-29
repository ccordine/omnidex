package api

import (
	"context"
	"encoding/json"
	"io"
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

func (s *Server) handleBrowseMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Parent string `json:"parent"`
		Name   string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if client := s.hostBridgeClient(); client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		path, err := client.Mkdir(ctx, req.Parent, req.Name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": path, "source": "host-bridge"})
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
	path, err := hostbridge.CreateDirectory(req.Parent, req.Name, opts)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "outside allowed browse roots") {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "source": "core-local"})
}
