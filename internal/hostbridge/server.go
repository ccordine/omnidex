package hostbridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Server struct {
	Token string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/browse", s.handleBrowse)
	mux.HandleFunc("/v1/mkdir", s.handleMkdir)
	mux.HandleFunc("/v1/pick-directory", s.handlePickDirectory)
	mux.HandleFunc("/v1/terminal/ws", s.handleTerminalWS)
	mux.HandleFunc("/v1/screen/monitors", s.handleScreenMonitors)
	mux.HandleFunc("/v1/screen/mjpeg", s.handleScreenMJPEG)
	mux.HandleFunc("/v1/cursor/run", s.handleCursorRun)
	mux.HandleFunc("/v1/codex/run", s.handleCodexRun)
	mux.HandleFunc("/v1/project-map/scan", s.handleProjectMapScan)
	mux.HandleFunc("/v1/project/git", s.handleProjectGit)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"service":       "omni-host-bridge",
		"native_picker": true,
		"mkdir":         true,
		"browse":        true,
		"terminal":      true,
		"screen":        true,
		"cursor":        true,
		"codex":         true,
		"project_map":   true,
		"project_git":   true,
	})
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	opts, err := browseRequestOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := ListDirectory(target, opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":            result.Path,
		"parent":          result.Parent,
		"entries":         NonEmptyEntries(result.Entries),
		"limit":           result.Limit,
		"offset":          result.Offset,
		"has_previous":    result.HasPrevious,
		"previous_offset": result.PreviousOffset,
		"has_more":        result.HasMore,
		"next_offset":     result.NextOffset,
	})
}

func browseRequestOptions(r *http.Request) (BrowseOptions, error) {
	limit, err := browseQueryInteger(r, "limit", DefaultBrowsePageSize)
	if err != nil {
		return BrowseOptions{}, err
	}
	offset, err := browseQueryInteger(r, "offset", 0)
	if err != nil {
		return BrowseOptions{}, err
	}
	directoriesOnly := false
	if raw := strings.TrimSpace(r.URL.Query().Get("directories_only")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return BrowseOptions{}, fmt.Errorf("directories_only must be a boolean")
		}
		directoriesOnly = value
	}
	opts := BrowseOptions{Limit: limit, Offset: offset, DirectoriesOnly: directoriesOnly}
	if err := validateBrowseBounds(opts); err != nil {
		return BrowseOptions{}, err
	}
	return opts, nil
}

func browseQueryInteger(r *http.Request, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
	path, err := CreateDirectory(req.Parent, req.Name, BrowseOptions{})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

func (s *Server) handlePickDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		StartPath string `json:"start_path"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	}
	path, err := PickDirectory(r.Context(), strings.TrimSpace(req.StartPath))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "cancel") {
			writeJSON(w, http.StatusOK, map[string]any{"canceled": true})
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

func (s *Server) authorize(r *http.Request) bool {
	token := strings.TrimSpace(s.Token)
	if token == "" {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:]) == token
	}
	if queryToken := strings.TrimSpace(r.URL.Query().Get("token")); queryToken != "" {
		return queryToken == token
	}
	return strings.TrimSpace(r.Header.Get("X-Omni-Host-Token")) == token
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(message)})
}
