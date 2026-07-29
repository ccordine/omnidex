package hostbridge

import (
	"encoding/json"
	"io"
	"net/http"
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
		Path     string `json:"path"`
		MaxFiles int    `json:"max_files"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid project scan request: "+err.Error())
		return
	}
	walk, err := WalkProjectTree(req.Path, req.MaxFiles)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"walk": walk, "message": "current project tree scanned"})
}
