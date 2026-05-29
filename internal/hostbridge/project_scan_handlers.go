package hostbridge

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
)

func (s *Server) handleProjectMapScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Path      string          `json:"path"`
		MaxFiles  int             `json:"max_files"`
		IndexJSON json.RawMessage `json:"index_json"`
		MapJSON   json.RawMessage `json:"map_json"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if len(req.IndexJSON) > 0 && len(req.MapJSON) > 0 {
		indexPath, mapPath, err := WriteProjectArtifacts(req.Path, req.IndexJSON, req.MapJSON)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"index_path": indexPath,
			"map_path":   mapPath,
			"message":    "codebase map persisted",
		})
		return
	}

	walk, err := WalkProjectTree(req.Path, req.MaxFiles)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"walk":    walk,
		"message": "project tree scanned",
	})
}

func (s *Server) handleProjectMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	blob, mapPath, err := ReadProjectMapFile(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	exists := false
	if _, err := os.Stat(mapPath); err == nil {
		exists = true
	}

	payload, err := decodeProjectMapBlob(blob)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"map_path": mapPath,
		"map":      payload,
		"exists":   exists,
	})
}
