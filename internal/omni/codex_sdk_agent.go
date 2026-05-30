package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
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
	if err := a.ensureRunner(context.Background()); err != nil {
		return nil, err
	}
	return &externalAgentCommandSession{
		agent: "codex",
		command: func(ctx context.Context, job ExternalAgentJob) (*exec.Cmd, error) {
			workspace := strings.TrimSpace(job.Workspace)
			if workspace == "" {
				workspace = "."
			}
			request := codexSDKRunnerRequest{
				APIKey:          a.APIKey,
				Model:           firstNonEmpty(a.Model, "gpt-5.3-codex"),
				Workspace:       workspace,
				CodexPath:       firstNonEmpty(a.CodexBin, "codex"),
				Prompt:          job.Prompt,
				ReasoningEffort: a.ReasoningEffort,
				SandboxMode:     a.SandboxMode,
				ApprovalPolicy:  a.ApprovalPolicy,
				NetworkAccess:   a.NetworkAccess,
				WebSearchMode:   a.WebSearchMode,
			}
			reqPath, err := writeExternalAgentRequest("omnidex-codex-sdk-request-*.json", request)
			if err != nil {
				return nil, err
			}
			cmd := exec.CommandContext(ctx, firstNonEmpty(a.NodeBin, "node"), filepath.Join(a.RunnerDir, "runner.mjs"), reqPath)
			cmd.Dir = a.RunnerDir
			return cmd, nil
		},
	}, nil
}

type codexSDKRunnerRequest struct {
	APIKey          string `json:"api_key,omitempty"`
	Model           string `json:"model"`
	Workspace       string `json:"workspace"`
	CodexPath       string `json:"codex_path"`
	Prompt          string `json:"prompt"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	SandboxMode     string `json:"sandbox_mode,omitempty"`
	ApprovalPolicy  string `json:"approval_policy,omitempty"`
	NetworkAccess   string `json:"network_access,omitempty"`
	WebSearchMode   string `json:"web_search_mode,omitempty"`
}

func (a *CodexSDKArchitectAgent) ensureRunner(ctx context.Context) error {
	if err := os.MkdirAll(a.RunnerDir, 0o755); err != nil {
		return fmt.Errorf("create codex sdk runner dir: %w", err)
	}
	packageJSON := filepath.Join(a.RunnerDir, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		if err := os.WriteFile(packageJSON, []byte(codexSDKRunnerPackageJSON), 0o644); err != nil {
			return fmt.Errorf("write codex sdk runner package.json: %w", err)
		}
	}
	runnerPath := filepath.Join(a.RunnerDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(codexSDKRunnerScript), 0o644); err != nil {
		return fmt.Errorf("write codex sdk runner script: %w", err)
	}
	if _, err := os.Stat(filepath.Join(a.RunnerDir, "node_modules", "@openai", "codex-sdk")); err == nil {
		return nil
	}
	installCtx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CODEX_INSTALL_TIMEOUT", 10*time.Minute))
	defer cancel()
	cmd := exec.CommandContext(installCtx, firstNonEmpty(a.NPMBin, "npm"), "install", "--silent", "--no-audit", "--no-fund")
	cmd.Dir = a.RunnerDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install @openai/codex-sdk runner dependency: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
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

const codexSDKRunnerPackageJSON = `{"type":"module","private":true,"dependencies":{"@openai/codex-sdk":"latest"}}
`

const codexSDKRunnerScript = `import { Codex } from "@openai/codex-sdk";
import fs from "node:fs/promises";

function emit(event) {
  console.log(JSON.stringify(event));
}

const requestPath = process.argv[2];
if (!requestPath) {
  throw new Error("request path is required");
}

const request = JSON.parse(await fs.readFile(requestPath, "utf8"));
const env = { ...process.env };
if (request.api_key) {
  env.OPENAI_API_KEY = request.api_key;
  env.CODEX_API_KEY = request.api_key;
}

function stringOption(value, fallback) {
  return typeof value === "string" && value.trim() ? value.trim() : fallback;
}

function booleanOption(value) {
  if (typeof value === "boolean") return value;
  if (typeof value !== "string") return undefined;
  const normalized = value.trim().toLowerCase();
  if (["1", "true", "yes", "on", "enabled"].includes(normalized)) return true;
  if (["0", "false", "no", "off", "disabled"].includes(normalized)) return false;
  return undefined;
}

const threadOptions = {
  workingDirectory: request.workspace || process.cwd(),
  skipGitRepoCheck: true,
  sandboxMode: stringOption(request.sandbox_mode, "workspace-write"),
  approvalPolicy: stringOption(request.approval_policy, "never"),
  model: request.model || "gpt-5.3-codex",
};
if (stringOption(request.reasoning_effort, "")) {
  threadOptions.modelReasoningEffort = stringOption(request.reasoning_effort, "");
}
if (stringOption(request.web_search_mode, "")) {
  threadOptions.webSearchMode = stringOption(request.web_search_mode, "");
}
const networkAccess = booleanOption(request.network_access);
if (networkAccess !== undefined) {
  threadOptions.networkAccessEnabled = networkAccess;
}

const codex = new Codex({
  codexPathOverride: request.codex_path || "codex",
  env,
});

const thread = codex.startThread(threadOptions);

emit({ agent: "codex", type: "started", message: "Codex external implementation session started" });

const { events } = await thread.runStreamed(request.prompt, {
  outputSchema: {
    type: "object",
    properties: {
      changed_files: { type: "array", items: { type: "string" } },
      summary: { type: "string" },
      commands_run: { type: "array", items: { type: "string" } },
      risks: { type: "array", items: { type: "string" } }
    },
    required: ["changed_files", "summary", "commands_run", "risks"],
    additionalProperties: false
  }
});

const items = [];
let finalResponse = "";
for await (const event of events) {
  if (event.type === "item.completed" || event.type === "item.updated" || event.type === "item.started") {
    const item = event.item || {};
    items.push(item);
    if (item.type === "command_execution") {
      emit({ agent: "codex", type: "command", message: item.status || "command", command: item.command || "", raw: item });
    } else if (item.type === "file_change") {
      emit({ agent: "codex", type: "file_change", message: item.status || "file change", files: (item.changes || []).map((change) => change.path), raw: item });
    } else if (item.type === "agent_message") {
      finalResponse = item.text || finalResponse;
      emit({ agent: "codex", type: "message", message: item.text || "", raw: item });
    } else if (item.type === "error") {
      emit({ agent: "codex", type: "error", message: item.message || "Codex item error", raw: item });
    } else {
      emit({ agent: "codex", type: item.type || "message", message: item.text || item.status || "", raw: item });
    }
  } else if (event.type === "turn.completed") {
    emit({ agent: "codex", type: "turn.completed", message: "Codex turn completed", raw: event });
  } else if (event.type === "turn.failed") {
    emit({ agent: "codex", type: "error", message: event.error?.message || "Codex turn failed", raw: event });
  }
}

emit({
  agent: "codex",
  type: "completed",
  message: finalResponse || "Codex external implementation session completed",
  evidence: items.map((item) => JSON.stringify(item)),
  raw: { thread_id: thread.id || "" }
});
`
