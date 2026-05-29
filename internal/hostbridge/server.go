package hostbridge

import (
	"encoding/json"
	"io"
	"net/http"
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
	mux.HandleFunc("/v1/project-map", s.handleProjectMap)
	mux.HandleFunc("/v1/project-map/scan", s.handleProjectMapScan)
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
		"project_map":   true,
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
	result, err := ListDirectory(target, BrowseOptions{})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    result.Path,
		"parent":  result.Parent,
		"entries": NonEmptyEntries(result.Entries),
	})
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
