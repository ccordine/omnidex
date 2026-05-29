package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (s *Server) hostBridgeClient() *hostbridge.Client {
	if strings.TrimSpace(s.hostAgentURL) == "" {
		return nil
	}
	timeout := 10 * time.Second
	if s.requestTimeout > 0 && s.requestTimeout < timeout {
		timeout = s.requestTimeout
	}
	return hostbridge.NewClient(s.hostAgentURL, s.hostAgentToken, timeout)
}

func (s *Server) handleHostBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.collectHostBridgeStatus(ctx))
}

func (s *Server) handleHostPickDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.hostBridgeClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: run `omni host serve` on the host and set HOST_AGENT_URL (for Docker: http://host.docker.internal:8091)")
		return
	}
	var req struct {
		StartPath string `json:"start_path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	result, err := client.PickDirectory(ctx, req.StartPath)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if result.Canceled {
		writeJSON(w, http.StatusOK, map[string]any{"canceled": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": result.Path})
}
