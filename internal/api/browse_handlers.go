package api

import (
	"context"
	"net/http"
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
	opts, err := browsePageOptions(r, hostbridge.DefaultBrowsePageSize, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if client := s.hostBridgeClient(); client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		result, err := client.Browse(ctx, target, opts)
		if err != nil {
			writeError(w, http.StatusBadGateway, hostBridgeAPIError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":            result.Path,
			"parent":          result.Parent,
			"entries":         hostbridge.NonEmptyEntries(result.Entries),
			"limit":           result.Limit,
			"offset":          result.Offset,
			"has_previous":    result.HasPrevious,
			"previous_offset": result.PreviousOffset,
			"has_more":        result.HasMore,
			"next_offset":     result.NextOffset,
			"source":          "host-bridge",
		})
		return
	}

	opts, err = s.projectAuthorizedBrowseOptions(r.Context(), target, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
		"path":            result.Path,
		"parent":          result.Parent,
		"entries":         hostbridge.NonEmptyEntries(result.Entries),
		"limit":           result.Limit,
		"offset":          result.Offset,
		"has_previous":    result.HasPrevious,
		"previous_offset": result.PreviousOffset,
		"has_more":        result.HasMore,
		"next_offset":     result.NextOffset,
		"source":          "core-local",
	})
}

func (s *Server) handleBrowseMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeBrowseMkdirRequest(w, r)
	if err != nil {
		writeError(w, browseMkdirRequestStatus(err), err.Error())
		return
	}
	if client := s.hostBridgeClient(); client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		path, err := client.Mkdir(ctx, req.Parent, req.Name)
		if err != nil {
			writeError(w, http.StatusBadGateway, hostBridgeAPIError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": path, "source": "host-bridge"})
		return
	}

	opts, err := s.projectAuthorizedBrowseOptions(r.Context(), req.Parent, hostbridge.BrowseOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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

func hostBridgeAPIError(err error) string {
	if err == nil {
		return "host bridge request failed"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "host bridge request failed"
	}
	if strings.Contains(message, "invalid host bridge JSON") ||
		strings.Contains(message, "404 page not found") ||
		strings.Contains(message, "method not allowed") {
		return message + " — rebuild/restart the host bridge (`omni host service install` or `omni host serve --listen 0.0.0.0:8091`)"
	}
	return message
}
