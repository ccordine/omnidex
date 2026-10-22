package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func (s *Server) hostBridgeClient() *hostbridge.Client {
	if strings.TrimSpace(s.hostAgentURL) == "" {
		return nil
	}
	return hostbridge.NewClient(s.hostAgentURL, s.hostAgentToken)
}

func (s *Server) handleHostBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	status := s.collectHostBridgeStatus(ctx)
	bundle, err := renderChatHostBridgeStatusBundle(status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status.HTML.Bundle = bundle
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleHostPickDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.hostBridgeClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: configure one exact HOST_AGENT_URL")
		return
	}
	var req struct {
		StartPath string `json:"start_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
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
