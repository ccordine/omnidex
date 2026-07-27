package omni

import (
	"context"
	"encoding/json"
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

type CodexSDKArchitectAgent struct {
	APIKey    string
	Model     string
	RunnerDir string
	NodeBin   string
	NPMBin    string
	CodexBin  string

	ReasoningEffort string
	SandboxMode     string
	ApprovalPolicy  string
	NetworkAccess   string
	WebSearchMode   string
}

// NewCodexSDKArchitectAgent returns the Codex SDK agent when enabled.
// Pass explicit=true when a card/project/workspace chose Codex — only an API key is required.
func NewCodexSDKArchitectAgent(explicit ...bool) *CodexSDKArchitectAgent {
	explicitRequest := len(explicit) > 0 && explicit[0]
	return newCodexSDKArchitectAgent(true, explicitRequest)
}

func NewCodexSDKArchitectAgentFromEnv() *CodexSDKArchitectAgent {
	return newCodexSDKArchitectAgent(false, false)
}

func newCodexSDKArchitectAgent(force, explicitRequest bool) *CodexSDKArchitectAgent {
	if !force && !externalArchitectAgentSelectedFromEnv("codex") {
		return nil
	}
	if !CodexSDKEnabled(explicitRequest) {
		return nil
	}
	runnerDir := strings.TrimSpace(os.Getenv("OMNI_CODEX_SDK_RUNNER_DIR"))
	if runnerDir == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
			runnerDir = filepath.Join(cacheDir, "omnidex", "codex-sdk-runner")
		} else {
			runnerDir = filepath.Join(os.TempDir(), "omnidex-codex-sdk-runner")
		}
	}
	return &CodexSDKArchitectAgent{
		APIKey:    secrets.CodexAPIKey(),
		Model:     firstNonEmpty(os.Getenv("OMNI_CODEX_MODEL"), "gpt-5.3-codex"),
		RunnerDir: runnerDir,
		NodeBin:   firstNonEmpty(os.Getenv("OMNI_CODEX_NODE_BIN"), "node"),
		NPMBin:    firstNonEmpty(os.Getenv("OMNI_CODEX_NPM_BIN"), "npm"),
		CodexBin:  firstNonEmpty(os.Getenv("OMNI_CODEX_BIN"), "codex"),

		ReasoningEffort: firstNonEmpty(os.Getenv("OMNI_CODEX_REASONING_EFFORT"), os.Getenv("OMNI_CODEX_MODEL_REASONING_EFFORT")),
		SandboxMode:     os.Getenv("OMNI_CODEX_SANDBOX_MODE"),
		ApprovalPolicy:  os.Getenv("OMNI_CODEX_APPROVAL_POLICY"),
		NetworkAccess:   os.Getenv("OMNI_CODEX_NETWORK_ACCESS"),
		WebSearchMode:   os.Getenv("OMNI_CODEX_WEB_SEARCH_MODE"),
	}
}

func (a *CodexSDKArchitectAgent) ApplyConfig(cfg agentconfig.Config) {
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

func externalArchitectAgentSelectedFromEnv(agent string) bool {
	selected := strings.ToLower(strings.TrimSpace(os.Getenv("OMNI_ARCHITECT_AGENT")))
	if selected == "" || selected == "none" || selected == "local" || selected == "omnidex" {
		return false
	}
	return selected == strings.ToLower(strings.TrimSpace(agent))
}

func (a *CodexSDKArchitectAgent) ArchitectAgentAvailable() (bool, string) {
	if a == nil {
		return false, "codex sdk architect agent is not configured"
	}
	if UseHostBridgeExternalAgents() && hostBridgeClientFromEnv() == nil {
		return false, "HOST_AGENT_URL is not configured; Codex runs on the host machine via the bridge when core is in Docker"
	}
	if !UseHostBridgeExternalAgents() {
		if err := codexrunner.PreflightError(codexrunner.PreflightFor(a.NodeBin, a.NPMBin, a.CodexBin)); err != nil {
			return false, err.Error()
		}
	}
	return true, ""
}

func (a *CodexSDKArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	if input.Packet.Mode == "" {
		input.Packet = buildCursorImplementationPacket(input.UserPrompt, input.ToolTask, input.ArchitectContract, structuredCommandDecisionRunConfig{CurrentWorkingDirectory: input.Workspace}, input.WorksiteSurvey)
	}
	session, err := a.NewExternalAgentSession(input)
	if err != nil {
		return CursorArchitectAgentResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CODEX_TIMEOUT", 90*time.Minute))
	defer cancel()
	result, err := StreamExternalAgentSession(ctx, session, ExternalAgentJob{
		SessionID: "codex-sdk",
		Agent:     "codex",
		Mode:      "implementation",
		Packet:    input.Packet,
		Prompt:    buildCodexArchitectPrompt(input),
		Workspace: input.Workspace,
	}, nil)
	return result, err
}

func (a *CodexSDKArchitectAgent) NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error) {
	if a == nil {
		return nil, fmt.Errorf("codex sdk architect agent is not configured")
	}
	if UseHostBridgeExternalAgents() {
		return newHostBridgeExternalAgentSessionWithOptions("codex", a.APIKey, firstNonEmpty(a.Model, "gpt-5.3-codex"), firstNonEmpty(a.CodexBin, "codex"), ExternalAgentRuntimeOptions{
			ReasoningEffort: a.ReasoningEffort,
			SandboxMode:     a.SandboxMode,
			ApprovalPolicy:  a.ApprovalPolicy,
			NetworkAccess:   a.NetworkAccess,
			WebSearchMode:   a.WebSearchMode,
		})
	}
	codexBin := firstNonEmpty(a.CodexBin, "codex")
	if err := codexrunner.PreflightError(codexrunner.PreflightFor(a.NodeBin, a.NPMBin, codexBin)); err != nil {
		return nil, err
	}
	codexPath, err := codexrunner.ResolveCodexPath(codexBin)
	if err != nil {
		return nil, err
	}
	if err := codexrunner.EnsureWithBins(context.Background(), a.RunnerDir, a.NPMBin); err != nil {
		return nil, err
	}
	return &externalAgentCommandSession{
		agent: "codex",
		command: func(ctx context.Context, job ExternalAgentJob) (*exec.Cmd, func() error, error) {
			workspace := strings.TrimSpace(job.Workspace)
			if workspace == "" {
				workspace = "."
			}
			request := codexrunner.Request{
				APIKey:          a.APIKey,
				Model:           firstNonEmpty(a.Model, "gpt-5.3-codex"),
				Workspace:       workspace,
				CodexPath:       codexPath,
				Prompt:          job.Prompt,
				ReasoningEffort: a.ReasoningEffort,
				SandboxMode:     a.SandboxMode,
				ApprovalPolicy:  a.ApprovalPolicy,
				NetworkAccess:   a.NetworkAccess,
				WebSearchMode:   a.WebSearchMode,
			}
			reqPath, err := writeExternalAgentRequest("omnidex-codex-sdk-request-*.json", request)
			if err != nil {
				return nil, nil, err
			}
			cmd, err := codexrunner.CommandWithBins(ctx, a.RunnerDir, reqPath, a.NodeBin)
			if err != nil {
				return nil, nil, errors.Join(err, removeExternalAgentRequest(reqPath))
			}
			return cmd, func() error { return removeExternalAgentRequest(reqPath) }, nil
		},
	}, nil
}

func buildCodexArchitectPrompt(input CursorArchitectAgentInput) string {
	payload := struct {
		Role         string                         `json:"role"`
		Packet       CursorImplementationPacket     `json:"codex_packet"`
		Observations []StructuredCommandObservation `json:"recent_observations,omitempty"`
		Rules        []string                       `json:"rules"`
	}{
		Role:         "codex_sdk_external_coder",
		Packet:       input.Packet,
		Observations: input.Observations,
		Rules: []string{
			"Act only as the bounded Codex implementation pilot for Omnidex.",
			"Use codex_packet as the authoritative mission packet; do not reinterpret the user's task beyond it.",
			"Edit only files in codex_packet.edit_surface under codex_packet.target_root.",
			"Treat codex_packet.read_only_context as read-only.",
			"Respect codex_packet.forbidden exactly.",
			"Your output is implementation evidence only; Omnidex will run proof commands, artifact validation, scope validation, and decide completion.",
			"Return the requested summary fields only: changed files, summary, commands run, and risks.",
		},
	}
	blob, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return input.UserPrompt
	}
	return string(blob)
}
