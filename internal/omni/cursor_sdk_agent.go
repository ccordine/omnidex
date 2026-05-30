package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/cursorrunner"
	"github.com/gryph/omnidex/internal/secrets"
)

type CursorSDKArchitectAgent struct {
	APIKey    string
	Model     string
	RunnerDir string
	NodeBin   string
	NPMBin    string
}

// NewCursorSDKArchitectAgent returns the Cursor SDK agent when enabled.
// Pass explicit=true when a card/project/workspace chose Cursor — only an API key is required.
func NewCursorSDKArchitectAgent(explicit ...bool) *CursorSDKArchitectAgent {
	explicitRequest := len(explicit) > 0 && explicit[0]
	return newCursorSDKArchitectAgent(true, explicitRequest)
}

func NewCursorSDKArchitectAgentFromEnv() *CursorSDKArchitectAgent {
	return newCursorSDKArchitectAgent(false, false)
}

func newCursorSDKArchitectAgent(force, explicitRequest bool) *CursorSDKArchitectAgent {
	if !force && !externalArchitectAgentSelectedFromEnv("cursor") {
		return nil
	}
	if !CursorSDKEnabled(explicitRequest) {
		return nil
	}
	apiKey := strings.TrimSpace(secrets.Lookup("cursor_api_key"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("CURSOR_API_KEY"))
	}
	if apiKey == "" {
		return nil
	}
	runnerDir := strings.TrimSpace(os.Getenv("OMNI_CURSOR_SDK_RUNNER_DIR"))
	if runnerDir == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
			runnerDir = filepath.Join(cacheDir, "omnidex", "cursor-sdk-runner")
		} else {
			runnerDir = filepath.Join(os.TempDir(), "omnidex-cursor-sdk-runner")
		}
	}
	return &CursorSDKArchitectAgent{
		APIKey:    apiKey,
		Model:     cursorrunner.DefaultModel(),
		RunnerDir: runnerDir,
		NodeBin:   firstNonEmpty(os.Getenv("OMNI_CURSOR_NODE_BIN"), "node"),
		NPMBin:    firstNonEmpty(os.Getenv("OMNI_CURSOR_NPM_BIN"), "npm"),
	}
}

func (a *CursorSDKArchitectAgent) ArchitectAgentAvailable() (bool, string) {
	if a == nil {
		return false, "cursor sdk architect agent is not configured"
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return false, "CURSOR_API_KEY is required for cursor sdk architect delegation"
	}
	if UseHostBridgeExternalAgents() && hostBridgeClientFromEnv() == nil {
		return false, "HOST_AGENT_URL is not configured; Cursor runs on the host machine via the bridge when core is in Docker"
	}
	return true, ""
}

func (a *CursorSDKArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	session, err := a.NewExternalAgentSession(input)
	if err != nil {
		return CursorArchitectAgentResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CURSOR_TIMEOUT", 90*time.Minute))
	defer cancel()
	result, err := StreamExternalAgentSession(ctx, session, ExternalAgentJob{
		SessionID: "cursor-sdk",
		Agent:     "cursor",
		Mode:      "implementation",
		Packet:    input.Packet,
		Prompt:    buildCursorArchitectPrompt(input),
		Workspace: input.Workspace,
	}, nil)
	return result, err
}

func (a *CursorSDKArchitectAgent) NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error) {
	if a == nil {
		return nil, fmt.Errorf("cursor sdk architect agent is not configured")
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, fmt.Errorf("CURSOR_API_KEY is required for cursor sdk architect delegation")
	}
	if UseHostBridgeExternalAgents() {
		return newHostBridgeExternalAgentSession("cursor", a.APIKey, firstNonEmpty(a.Model, cursorrunner.DefaultModel()), "")
	}
	if err := cursorrunner.Ensure(context.Background(), a.RunnerDir); err != nil {
		return nil, err
	}
	return &externalAgentCommandSession{
		agent: "cursor",
		command: func(ctx context.Context, job ExternalAgentJob) (*exec.Cmd, error) {
			workspace := strings.TrimSpace(job.Workspace)
			if workspace == "" {
				workspace = "."
			}
			request := cursorSDKRunnerRequest{
				APIKey:    a.APIKey,
				Model:     firstNonEmpty(a.Model, cursorrunner.DefaultModel()),
				Workspace: workspace,
				Prompt:    job.Prompt,
			}
			reqPath, err := writeExternalAgentRequest("omnidex-cursor-sdk-request-*.json", request)
			if err != nil {
				return nil, err
			}
			cmd := exec.CommandContext(ctx, firstNonEmpty(a.NodeBin, "node"), filepath.Join(a.RunnerDir, "runner.mjs"), reqPath)
			cmd.Dir = a.RunnerDir
			cmd.Env = cursorrunner.CommandEnv()
			return cmd, nil
		},
	}, nil
}

type cursorSDKRunnerRequest struct {
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Prompt    string `json:"prompt"`
}

func buildCursorArchitectPrompt(input CursorArchitectAgentInput) string {
	payload := struct {
		Role         string                         `json:"role"`
		Packet       CursorImplementationPacket     `json:"cursor_packet"`
		Observations []StructuredCommandObservation `json:"recent_observations,omitempty"`
		Rules        []string                       `json:"rules"`
	}{
		Role:         "cursor_sdk_external_coder",
		Packet:       input.Packet,
		Observations: input.Observations,
		Rules: []string{
			"Act only as the bounded implementation pilot for Omnidex.",
			"Use cursor_packet as the authoritative mission packet; do not reinterpret the user's task beyond it.",
			"Edit only files in cursor_packet.edit_surface under cursor_packet.target_root.",
			"Treat cursor_packet.read_only_context as read-only.",
			"Respect cursor_packet.forbidden exactly.",
			"You may run proof commands if useful, but your output is implementation evidence only; Omnidex decides completion after independent validation.",
			"Return the requested summary fields only: files changed, implementation summary, commands run if any, and known risks.",
		},
	}
	blob, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return input.UserPrompt
	}
	return string(blob)
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

