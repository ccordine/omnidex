package cursorrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const PackageJSON = `{"type":"module","private":true,"dependencies":{"@cursor/sdk":"latest"}}
`

const RunnerScript = `import { Agent } from "@cursor/sdk";
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

type Request struct {
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Prompt    string `json:"prompt"`
}

func DefaultRunnerDir() string {
	if custom := strings.TrimSpace(os.Getenv("OMNI_CURSOR_SDK_RUNNER_DIR")); custom != "" {
		return custom
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "omnidex", "cursor-sdk-runner")
	}
	return filepath.Join(os.TempDir(), "omnidex-cursor-sdk-runner")
}

func NodeBin() string {
	return firstNonEmpty(os.Getenv("OMNI_CURSOR_NODE_BIN"), "node")
}

func NPMBin() string {
	return firstNonEmpty(os.Getenv("OMNI_CURSOR_NPM_BIN"), "npm")
}

func Ensure(ctx context.Context, runnerDir string) error {
	runnerDir = strings.TrimSpace(runnerDir)
	if runnerDir == "" {
		runnerDir = DefaultRunnerDir()
	}
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		return fmt.Errorf("create cursor sdk runner dir: %w", err)
	}
	packageJSON := filepath.Join(runnerDir, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		if err := os.WriteFile(packageJSON, []byte(PackageJSON), 0o644); err != nil {
			return fmt.Errorf("write cursor sdk runner package.json: %w", err)
		}
	}
	runnerPath := filepath.Join(runnerDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(RunnerScript), 0o644); err != nil {
		return fmt.Errorf("write cursor sdk runner script: %w", err)
	}
	if _, err := os.Stat(filepath.Join(runnerDir, "node_modules", "@cursor", "sdk")); err == nil {
		return nil
	}
	if _, err := exec.LookPath(NPMBin()); err != nil {
		return fmt.Errorf("npm is not available in PATH (%s); install Node.js/npm on this machine or set OMNI_CURSOR_NPM_BIN", NPMBin())
	}
	installCtx, cancel := context.WithTimeout(ctx, installTimeout())
	defer cancel()
	cmd := exec.CommandContext(installCtx, NPMBin(), "install", "--silent", "--no-audit", "--no-fund")
	cmd.Dir = runnerDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install @cursor/sdk runner dependency: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func Command(ctx context.Context, runnerDir, requestPath string) (*exec.Cmd, error) {
	runnerDir = strings.TrimSpace(runnerDir)
	if runnerDir == "" {
		runnerDir = DefaultRunnerDir()
	}
	if _, err := exec.LookPath(NodeBin()); err != nil {
		return nil, fmt.Errorf("node is not available in PATH (%s)", NodeBin())
	}
	cmd := exec.CommandContext(ctx, NodeBin(), filepath.Join(runnerDir, "runner.mjs"), requestPath)
	cmd.Dir = runnerDir
	return cmd, nil
}

func installTimeout() time.Duration {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
