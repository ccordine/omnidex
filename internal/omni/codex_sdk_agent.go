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
	"github.com/gryph/omnidex/internal/codexrunner"
	"github.com/gryph/omnidex/internal/secrets"
)

type CodexSDKAgent struct {
	APIKey, Model, RunnerDir, NodeBin, NPMBin, CodexBin string
	ReasoningEffort, SandboxMode, ApprovalPolicy        string
	NetworkAccess, WebSearchMode                        string
}

func NewCodexSDKAgent() *CodexSDKAgent {
	if !CodexSDKEnabled() {
		return nil
	}
	runnerDir := strings.TrimSpace(os.Getenv("OMNI_CODEX_SDK_RUNNER_DIR"))
	if runnerDir == "" {
		runnerDir = filepath.Join(os.TempDir(), "omnidex-codex-sdk-runner")
		if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
			runnerDir = filepath.Join(cacheDir, "omnidex", "codex-sdk-runner")
		}
	}
	return &CodexSDKAgent{
		APIKey: secrets.CodexAPIKey(), Model: firstNonEmpty(os.Getenv("OMNI_CODEX_MODEL"), "gpt-5.3-codex"), RunnerDir: runnerDir,
		NodeBin: firstNonEmpty(os.Getenv("OMNI_CODEX_NODE_BIN"), "node"), NPMBin: firstNonEmpty(os.Getenv("OMNI_CODEX_NPM_BIN"), "npm"), CodexBin: firstNonEmpty(os.Getenv("OMNI_CODEX_BIN"), "codex"),
		ReasoningEffort: firstNonEmpty(os.Getenv("OMNI_CODEX_REASONING_EFFORT"), os.Getenv("OMNI_CODEX_MODEL_REASONING_EFFORT")),
		SandboxMode:     os.Getenv("OMNI_CODEX_SANDBOX_MODE"), ApprovalPolicy: os.Getenv("OMNI_CODEX_APPROVAL_POLICY"), NetworkAccess: os.Getenv("OMNI_CODEX_NETWORK_ACCESS"), WebSearchMode: os.Getenv("OMNI_CODEX_WEB_SEARCH_MODE"),
	}
}

func (a *CodexSDKAgent) ApplyConfig(cfg agentconfig.Config) {
	if a == nil {
		return
	}
	if value := cfg.CodexModel(); value != "" {
		a.Model = value
	}
	if value := cfg.CodexReasoningEffort(); value != "" {
		a.ReasoningEffort = value
	}
	if value := cfg.CodexSandboxMode(); value != "" {
		a.SandboxMode = value
	}
	if value := cfg.CodexApprovalPolicy(); value != "" {
		a.ApprovalPolicy = value
	}
	if value := cfg.CodexNetworkAccess(); value != "" {
		a.NetworkAccess = value
	}
	if value := cfg.CodexWebSearchMode(); value != "" {
		a.WebSearchMode = value
	}
}

func (a *CodexSDKAgent) Available() (bool, string) {
	if a == nil {
		return false, "Codex SDK agent is not configured"
	}
	if UseHostBridgeExternalAgents() {
		if hostBridgeClientFromEnv() == nil {
			return false, "HOST_AGENT_URL is required for host-bridge Codex execution"
		}
		return true, ""
	}
	if err := codexrunner.PreflightError(codexrunner.PreflightFor(a.NodeBin, a.NPMBin, a.CodexBin)); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (a *CodexSDKAgent) RunCodingTask(ctx context.Context, request ExternalCodingRequest) (ExternalCodingResult, error) {
	timeout, err := externalAgentTimeout("OMNI_CODEX_TIMEOUT", 90*time.Minute)
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

func (a *CodexSDKAgent) PrepareCodingSession(request ExternalCodingRequest) (ExternalAgentSession, ExternalAgentJob, error) {
	prompt, err := buildExternalCodingPrompt("codex", request)
	if err != nil {
		return nil, ExternalAgentJob{}, err
	}
	session, err := a.newExternalAgentSession()
	if err != nil {
		return nil, ExternalAgentJob{}, err
	}
	job := ExternalAgentJob{SessionID: "codex-sdk", Agent: "codex", Prompt: prompt, Workspace: request.Workspace}
	return session, job, nil
}

func (a *CodexSDKAgent) newExternalAgentSession() (ExternalAgentSession, error) {
	if ok, reason := a.Available(); !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	if UseHostBridgeExternalAgents() {
		return newHostBridgeExternalAgentSessionWithOptions("codex", a.APIKey, firstNonEmpty(a.Model, "gpt-5.3-codex"), firstNonEmpty(a.CodexBin, "codex"), ExternalAgentRuntimeOptions{ReasoningEffort: a.ReasoningEffort, SandboxMode: a.SandboxMode, ApprovalPolicy: a.ApprovalPolicy, NetworkAccess: a.NetworkAccess, WebSearchMode: a.WebSearchMode})
	}
	codexPath, err := codexrunner.ResolveCodexPath(a.CodexBin)
	if err != nil {
		return nil, err
	}
	if err := codexrunner.EnsureWithBins(context.Background(), a.RunnerDir, a.NPMBin); err != nil {
		return nil, err
	}
	return &externalAgentCommandSession{agent: "codex", command: func(ctx context.Context, job ExternalAgentJob) (*exec.Cmd, func() error, error) {
		request := codexrunner.Request{APIKey: a.APIKey, Model: firstNonEmpty(a.Model, "gpt-5.3-codex"), Workspace: job.Workspace, CodexPath: codexPath, Prompt: job.Prompt, ReasoningEffort: a.ReasoningEffort, SandboxMode: a.SandboxMode, ApprovalPolicy: a.ApprovalPolicy, NetworkAccess: a.NetworkAccess, WebSearchMode: a.WebSearchMode}
		path, err := writeExternalAgentRequest("omnidex-codex-sdk-request-*.json", request)
		if err != nil {
			return nil, nil, err
		}
		cmd, err := codexrunner.CommandWithBins(ctx, a.RunnerDir, path, a.NodeBin)
		if err != nil {
			return nil, nil, errors.Join(err, removeExternalAgentRequest(path))
		}
		return cmd, func() error { return removeExternalAgentRequest(path) }, nil
	}}, nil
}
