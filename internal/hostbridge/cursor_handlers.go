package hostbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/cursorrunner"
)

type cursorRunRequest struct {
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Prompt    string `json:"prompt"`
}

func (s *Server) handleCursorRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req cursorRunRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(req.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	workspace, err := validateHostWorkspace(strings.TrimSpace(req.Workspace))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runnerDir := cursorrunner.DefaultRunnerDir()
	setupCtx, cancel := context.WithTimeout(r.Context(), cursorInstallTimeout())
	defer cancel()
	if err := cursorrunner.Ensure(setupCtx, runnerDir); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	request := cursorrunner.Request{
		APIKey:    strings.TrimSpace(req.APIKey),
		Model:     firstNonEmpty(req.Model, "composer-2"),
		Workspace: workspace,
		Prompt:    strings.TrimSpace(req.Prompt),
	}
	reqPath, err := writeTempJSONRequest("omnidex-cursor-sdk-request-*.json", request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(reqPath)

	runCtx, runCancel := context.WithTimeout(r.Context(), cursorRunTimeout())
	defer runCancel()
	cmd, err := cursorrunner.Command(runCtx, runnerDir, reqPath)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	streamCommandNDJSON(w, cmd, "cursor")
}

func cursorInstallTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("OMNI_CURSOR_INSTALL_TIMEOUT"))
	if value == "" {
		return 10 * time.Minute
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 10 * time.Minute
	}
	return parsed
}

func cursorRunTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("OMNI_CURSOR_TIMEOUT"))
	if value == "" {
		return 90 * time.Minute
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 90 * time.Minute
	}
	return parsed
}
