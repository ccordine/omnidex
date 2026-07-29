package omni

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/cursorrunner"
	"github.com/gryph/omnidex/internal/secrets"
)

type CursorSDKAgent struct {
	APIKey    string
	Model     string
	RunnerDir string
	NodeBin   string
	NPMBin    string
}

func NewCursorSDKAgent() *CursorSDKAgent {
	if !CursorSDKEnabled() {
		return nil
	}
	apiKey := firstNonEmpty(secrets.Lookup("cursor_api_key"), os.Getenv("CURSOR_API_KEY"))
	if apiKey == "" {
		return nil
	}
	runnerDir := strings.TrimSpace(os.Getenv("OMNI_CURSOR_SDK_RUNNER_DIR"))
	if runnerDir == "" {
		runnerDir = filepath.Join(os.TempDir(), "omnidex-cursor-sdk-runner")
		if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
			runnerDir = filepath.Join(cacheDir, "omnidex", "cursor-sdk-runner")
		}
	}
	return &CursorSDKAgent{
		APIKey: apiKey, Model: cursorrunner.DefaultModel(), RunnerDir: runnerDir,
		NodeBin: firstNonEmpty(os.Getenv("OMNI_CURSOR_NODE_BIN"), "node"),
		NPMBin:  firstNonEmpty(os.Getenv("OMNI_CURSOR_NPM_BIN"), "npm"),
	}
}

func (a *CursorSDKAgent) ApplyConfig(cfg agentconfig.Config) {
	if a != nil && cfg.CursorModel() != "" {
		a.Model = cfg.CursorModel()
	}
}

func (a *CursorSDKAgent) Available() (bool, string) {
	if a == nil {
		return false, "Cursor SDK agent is not configured"
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return false, "CURSOR_API_KEY is required for Cursor SDK execution"
	}
	if UseHostBridgeExternalAgents() {
		if hostBridgeClientFromEnv() == nil {
			return false, "HOST_AGENT_URL is required for host-bridge Cursor execution"
		}
		return true, ""
	}
	if err := cursorrunner.PreflightError(cursorrunner.Preflight()); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (a *CursorSDKAgent) RunCodingTask(ctx context.Context, request ExternalCodingRequest) (ExternalCodingResult, error) {
	timeout, err := externalAgentTimeout("OMNI_CURSOR_TIMEOUT", 90*time.Minute)
	if err != nil {
		return ExternalCodingResult{}, err
	}
	session, job, err := a.PrepareCodingSession(request)
	if err != nil {
		return ExternalCodingResult{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return StreamExternalAgentSession(runCtx, session, job, nil)
}

func (a *CursorSDKAgent) PrepareCodingSession(request ExternalCodingRequest) (ExternalAgentSession, ExternalAgentJob, error) {
	prompt, err := buildExternalCodingPrompt("cursor", request)
	if err != nil {
		return nil, ExternalAgentJob{}, err
	}
	session, err := a.newExternalAgentSession()
	if err != nil {
		return nil, ExternalAgentJob{}, err
	}
	job := ExternalAgentJob{SessionID: "cursor-sdk", Agent: "cursor", Prompt: prompt, Workspace: request.Workspace}
	return session, job, nil
}

func (a *CursorSDKAgent) newExternalAgentSession() (ExternalAgentSession, error) {
	if ok, reason := a.Available(); !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	if UseHostBridgeExternalAgents() {
		return newHostBridgeExternalAgentSession("cursor", a.APIKey, firstNonEmpty(a.Model, cursorrunner.DefaultModel()), "")
	}
	if err := cursorrunner.Ensure(context.Background(), a.RunnerDir); err != nil {
		return nil, err
	}
	return &externalAgentCommandSession{agent: "cursor", command: func(ctx context.Context, job ExternalAgentJob) (*exec.Cmd, func() error, error) {
		request := cursorSDKRunnerRequest{APIKey: a.APIKey, Model: firstNonEmpty(a.Model, cursorrunner.DefaultModel()), Workspace: job.Workspace, Prompt: job.Prompt}
		path, err := writeExternalAgentRequest("omnidex-cursor-sdk-request-*.json", request)
		if err != nil {
			return nil, nil, err
		}
		cmd, err := cursorrunner.Command(ctx, a.RunnerDir, path)
		if err != nil {
			return nil, nil, errors.Join(err, removeExternalAgentRequest(path))
		}
		return cmd, func() error { return removeExternalAgentRequest(path) }, nil
	}}, nil
}

type cursorSDKRunnerRequest struct {
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Prompt    string `json:"prompt"`
}
