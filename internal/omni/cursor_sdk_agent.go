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
)

type CursorSDKArchitectAgent struct {
	APIKey    string
	Model     string
	RunnerDir string
	NodeBin   string
	NPMBin    string
}

func NewCursorSDKArchitectAgentFromEnv() *CursorSDKArchitectAgent {
	if !externalArchitectAgentSelectedFromEnv("cursor") || !envBoolOrDefault("OMNI_ENABLE_CURSOR_ARCHITECT", false) || envBoolOrDefault("OMNI_DISABLE_CURSOR_ARCHITECT", false) {
		return nil
	}
	apiKey := strings.TrimSpace(os.Getenv("CURSOR_API_KEY"))
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
		Model:     firstNonEmpty(os.Getenv("OMNI_CURSOR_MODEL"), "composer-2"),
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
	return true, ""
}

func (a *CursorSDKArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	session, err := a.NewExternalAgentSession(input)
	if err != nil {
		return CursorArchitectAgentResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CURSOR_TIMEOUT", 90*time.Minute))
	defer cancel()
	events, err := session.Start(ctx, ExternalAgentJob{SessionID: "cursor-sdk", Agent: "cursor", Mode: "implementation", Packet: input.Packet, Prompt: buildCursorArchitectPrompt(input), Workspace: input.Workspace})
	if err != nil {
		return CursorArchitectAgentResult{}, err
	}
	return resultFromExternalAgentEvents(events), nil
}

func (a *CursorSDKArchitectAgent) NewExternalAgentSession(input CursorArchitectAgentInput) (ExternalAgentSession, error) {
	if a == nil {
		return nil, fmt.Errorf("cursor sdk architect agent is not configured")
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, fmt.Errorf("CURSOR_API_KEY is required for cursor sdk architect delegation")
	}
	if err := a.ensureRunner(context.Background()); err != nil {
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
				Model:     firstNonEmpty(a.Model, "composer-2"),
				Workspace: workspace,
				Prompt:    job.Prompt,
			}
			reqPath, err := writeExternalAgentRequest("omnidex-cursor-sdk-request-*.json", request)
			if err != nil {
				return nil, err
			}
			cmd := exec.CommandContext(ctx, firstNonEmpty(a.NodeBin, "node"), filepath.Join(a.RunnerDir, "runner.mjs"), reqPath)
			cmd.Dir = a.RunnerDir
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

func (a *CursorSDKArchitectAgent) ensureRunner(ctx context.Context) error {
	if err := os.MkdirAll(a.RunnerDir, 0o755); err != nil {
		return fmt.Errorf("create cursor sdk runner dir: %w", err)
	}
	packageJSON := filepath.Join(a.RunnerDir, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		if err := os.WriteFile(packageJSON, []byte(cursorSDKRunnerPackageJSON), 0o644); err != nil {
			return fmt.Errorf("write cursor sdk runner package.json: %w", err)
		}
	}
	runnerPath := filepath.Join(a.RunnerDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(cursorSDKRunnerScript), 0o644); err != nil {
		return fmt.Errorf("write cursor sdk runner script: %w", err)
	}
	if _, err := os.Stat(filepath.Join(a.RunnerDir, "node_modules", "@cursor", "sdk")); err == nil {
		return nil
	}
	installCtx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CURSOR_INSTALL_TIMEOUT", 10*time.Minute))
	defer cancel()
	cmd := exec.CommandContext(installCtx, firstNonEmpty(a.NPMBin, "npm"), "install", "--silent", "--no-audit", "--no-fund")
	cmd.Dir = a.RunnerDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install @cursor/sdk runner dependency: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
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

const cursorSDKRunnerPackageJSON = `{"type":"module","private":true,"dependencies":{"@cursor/sdk":"latest"}}
`

const cursorSDKRunnerScript = `import { Agent } from "@cursor/sdk";
import fs from "node:fs/promises";

function emit(event) {
  console.log(JSON.stringify(event));
}

const requestPath = process.argv[2];
if (!requestPath) {
  throw new Error("request path is required");
}

const request = JSON.parse(await fs.readFile(requestPath, "utf8"));
const agent = await Agent.create({
  apiKey: request.api_key,
  model: { id: request.model || "composer-2" },
  local: { cwd: request.workspace || process.cwd() },
});

const run = await agent.send(request.prompt);
const events = [];
let summary = "";
let agentID = "";
let runID = "";

emit({ agent: "cursor", type: "started", message: "Cursor external implementation session started" });

for await (const event of run.stream()) {
  events.push(event);
  const text = typeof event === "string" ? event : JSON.stringify(event);
  if (text) {
    summary = text;
  }
  if (event && typeof event === "object") {
    agentID = agentID || event.agent_id || event.agentId || event.agent?.id || "";
    runID = runID || event.run_id || event.runId || event.run?.id || "";
  }
  emit({ agent: "cursor", type: "message", message: text, raw: event });
}

emit({
  agent: "cursor",
  type: "completed",
  message: summary || "Cursor external implementation session completed",
  evidence: events.map((event) => typeof event === "string" ? event : JSON.stringify(event)),
  raw: { agent_id: agentID, run_id: runID }
});
`
