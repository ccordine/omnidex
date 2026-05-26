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
	if envBoolOrDefault("OMNI_DISABLE_CURSOR_ARCHITECT", false) {
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

func (a *CursorSDKArchitectAgent) RunArchitectTask(ctx context.Context, input CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	if a == nil {
		return CursorArchitectAgentResult{}, fmt.Errorf("cursor sdk architect agent is not configured")
	}
	if strings.TrimSpace(a.APIKey) == "" {
		return CursorArchitectAgentResult{}, fmt.Errorf("CURSOR_API_KEY is required for cursor sdk architect delegation")
	}
	workspace := strings.TrimSpace(input.Workspace)
	if workspace == "" {
		workspace = "."
	}
	if err := a.ensureRunner(ctx); err != nil {
		return CursorArchitectAgentResult{}, err
	}
	request := cursorSDKRunnerRequest{
		APIKey:    a.APIKey,
		Model:     firstNonEmpty(a.Model, "composer-2"),
		Workspace: workspace,
		Prompt:    buildCursorArchitectPrompt(input),
	}
	blob, err := json.Marshal(request)
	if err != nil {
		return CursorArchitectAgentResult{}, fmt.Errorf("marshal cursor sdk request: %w", err)
	}
	reqFile, err := os.CreateTemp("", "omnidex-cursor-sdk-request-*.json")
	if err != nil {
		return CursorArchitectAgentResult{}, fmt.Errorf("create cursor sdk request: %w", err)
	}
	reqPath := reqFile.Name()
	defer os.Remove(reqPath)
	if _, err := reqFile.Write(blob); err != nil {
		reqFile.Close()
		return CursorArchitectAgentResult{}, fmt.Errorf("write cursor sdk request: %w", err)
	}
	if err := reqFile.Close(); err != nil {
		return CursorArchitectAgentResult{}, fmt.Errorf("close cursor sdk request: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, envDurationOrDefault("OMNI_CURSOR_TIMEOUT", 90*time.Minute))
	defer cancel()
	cmd := exec.CommandContext(runCtx, firstNonEmpty(a.NodeBin, "node"), filepath.Join(a.RunnerDir, "runner.mjs"), reqPath)
	cmd.Dir = a.RunnerDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return CursorArchitectAgentResult{}, fmt.Errorf("run cursor sdk architect agent: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var result CursorArchitectAgentResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return CursorArchitectAgentResult{Output: stdout.String()}, nil
	}
	return result, nil
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
		Role              string                          `json:"role"`
		UserPrompt        string                          `json:"user_prompt"`
		ToolTask          string                          `json:"tool_task"`
		ArchitectContract ImplementationArchitectContract `json:"architect_contract"`
		Observations      []StructuredCommandObservation  `json:"observations,omitempty"`
		SessionMemories   []SessionMemory                 `json:"session_memories,omitempty"`
		WorksiteSurvey    WorksiteSurvey                  `json:"worksite_survey"`
		Rules             []string                        `json:"rules"`
	}{
		Role:              "cursor_sdk_implementation_architect_agent",
		UserPrompt:        input.UserPrompt,
		ToolTask:          input.ToolTask,
		ArchitectContract: input.ArchitectContract,
		Observations:      input.Observations,
		SessionMemories:   input.SessionMemories,
		WorksiteSurvey:    input.WorksiteSurvey,
		Rules: []string{
			"You are the delegated Cursor coding agent for the implementation architect.",
			"Modify files only under architect_contract.target_root and the architect_contract.edit_surface unless a proof command requires generated dependency output.",
			"Complete the architect_contract.work_queue end to end: read existing files, write substantive source/tests, install dependencies only when queued or necessary for the package metadata, run proof commands, and validate the result.",
			"Do not create placeholder-only files, sibling projects, unrequested features, or unrelated refactors.",
			"Run the configured proof commands from target_root and fix failures before finishing.",
			"Finish with a concise summary of changed files and proof command results.",
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
}

console.log(JSON.stringify({
  summary,
  agent_id: agentID,
  run_id: runID,
  output: events.map((event) => typeof event === "string" ? event : JSON.stringify(event)).join("\n"),
}));
`
