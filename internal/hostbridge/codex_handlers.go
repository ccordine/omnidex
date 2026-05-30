package hostbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/codexrunner"
)

type codexRunRequest struct {
	APIKey    string `json:"api_key,omitempty"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	CodexPath string `json:"codex_path"`
	Prompt    string `json:"prompt"`
}

func (s *Server) handleCodexRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req codexRunRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
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

	runnerDir := codexrunner.DefaultRunnerDir()
	setupCtx, cancel := context.WithTimeout(r.Context(), codexInstallTimeout())
	defer cancel()
	if err := codexrunner.Ensure(setupCtx, runnerDir); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	request := codexrunner.Request{
		APIKey:    strings.TrimSpace(req.APIKey),
		Model:     firstNonEmpty(req.Model, "gpt-5.3-codex"),
		Workspace: workspace,
		CodexPath: firstNonEmpty(req.CodexPath, codexrunner.CodexBin()),
		Prompt:    strings.TrimSpace(req.Prompt),
	}
	reqPath, err := writeTempJSONRequest("omnidex-codex-sdk-request-*.json", request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(reqPath)

	runCtx, runCancel := context.WithTimeout(r.Context(), codexRunTimeout())
	defer runCancel()
	cmd, err := codexrunner.Command(runCtx, runnerDir, reqPath)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	streamCommandNDJSON(w, cmd, "codex")
}

func codexInstallTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("OMNI_CODEX_INSTALL_TIMEOUT"))
	if value == "" {
		return 10 * time.Minute
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 10 * time.Minute
	}
	return parsed
}

func codexRunTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("OMNI_CODEX_TIMEOUT"))
	if value == "" {
		return 90 * time.Minute
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 90 * time.Minute
	}
	return parsed
}
