package codexrunner

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

const PackageJSON = `{"type":"module","private":true,"dependencies":{"@openai/codex-sdk":"latest"}}
`

const RunnerScript = `import { Codex } from "@openai/codex-sdk";
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
  config: {
    show_raw_agent_reasoning: true,
  },
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
const seenItems = new Map();

function itemText(item) {
  if (!item || typeof item !== "object") return "";
  for (const key of ["text", "message", "summary", "content", "aggregated_output", "output"]) {
    const value = item[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function itemFiles(item) {
  if (!item || typeof item !== "object") return [];
  if (Array.isArray(item.files)) return item.files.filter(Boolean).map(String);
  if (Array.isArray(item.changes)) return item.changes.map((change) => change?.path).filter(Boolean).map(String);
  if (typeof item.path === "string" && item.path.trim()) return [item.path.trim()];
  return [];
}

function todoListText(item) {
  if (!Array.isArray(item?.items)) return "";
  return item.items
    .map((entry) => (entry?.completed ? "[x]" : "[ ]") + " " + String(entry?.text || "").trim())
    .filter((line) => line.trim() !== "[ ]" && line.trim() !== "[x]")
    .join("\n");
}

function shouldEmitItem(item, eventType) {
  if (!item?.id) return true;
  const current = JSON.stringify(item);
  const previous = seenItems.get(item.id);
  if (previous === current && eventType !== "item.completed") return false;
  seenItems.set(item.id, current);
  return true;
}

function emitCodexItem(item, eventType) {
  if (!item || typeof item !== "object" || !shouldEmitItem(item, eventType)) return;
  items.push(item);
  const status = item.status || (eventType === "item.started" ? "in_progress" : eventType === "item.completed" ? "completed" : "in_progress");
  if (item.type === "command_execution") {
    emit({ agent: "codex", type: "command", message: status, command: item.command || "", raw: item });
    if (item.aggregated_output) {
      emit({ agent: "codex", type: "tool", message: item.aggregated_output, raw: { ...item, type: "command_output" } });
    }
    return;
  }
  if (item.type === "file_change") {
    emit({ agent: "codex", type: "file_change", message: status, files: itemFiles(item), raw: item });
    return;
  }
  if (item.type === "mcp_tool_call") {
    emit({ agent: "codex", type: "tool", message: item.tool || item.server || "mcp tool", raw: item });
    return;
  }
  if (item.type === "web_search") {
    emit({ agent: "codex", type: "tool", message: item.query || "web search", raw: item });
    return;
  }
  if (item.type === "reasoning") {
    const text = itemText(item);
    if (text) emit({ agent: "codex", type: "thinking", message: text, raw: item });
    return;
  }
  if (item.type === "todo_list") {
    const text = todoListText(item);
    if (text) emit({ agent: "codex", type: "thinking", message: text, raw: item });
    return;
  }
  if (item.type === "agent_message") {
    finalResponse = item.text || finalResponse;
    emit({ agent: "codex", type: "message", message: item.text || "", raw: item });
    return;
  }
  if (item.type === "error") {
    emit({ agent: "codex", type: "error", message: item.message || "Codex item error", raw: item });
    return;
  }
  emit({ agent: "codex", type: item.type || "message", message: itemText(item) || status || "", raw: item });
}

for await (const event of events) {
  if (event.type === "item.completed" || event.type === "item.updated" || event.type === "item.started") {
    emitCodexItem(event.item || {}, event.type);
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

type Request struct {
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

func DefaultRunnerDir() string {
	if custom := strings.TrimSpace(os.Getenv("OMNI_CODEX_SDK_RUNNER_DIR")); custom != "" {
		return custom
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "omnidex", "codex-sdk-runner")
	}
	return filepath.Join(os.TempDir(), "omnidex-codex-sdk-runner")
}

func NodeBin() string {
	return firstNonEmpty(os.Getenv("OMNI_CODEX_NODE_BIN"), "node")
}

func NPMBin() string {
	return firstNonEmpty(os.Getenv("OMNI_CODEX_NPM_BIN"), "npm")
}

func CodexBin() string {
	return firstNonEmpty(os.Getenv("OMNI_CODEX_BIN"), "codex")
}

func Ensure(ctx context.Context, runnerDir string) error {
	runnerDir = strings.TrimSpace(runnerDir)
	if runnerDir == "" {
		runnerDir = DefaultRunnerDir()
	}
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		return fmt.Errorf("create codex sdk runner dir: %w", err)
	}
	packageJSON := filepath.Join(runnerDir, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		if err := os.WriteFile(packageJSON, []byte(PackageJSON), 0o644); err != nil {
			return fmt.Errorf("write codex sdk runner package.json: %w", err)
		}
	}
	runnerPath := filepath.Join(runnerDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(RunnerScript), 0o644); err != nil {
		return fmt.Errorf("write codex sdk runner script: %w", err)
	}
	if _, err := os.Stat(filepath.Join(runnerDir, "node_modules", "@openai", "codex-sdk")); err == nil {
		return nil
	}
	if _, err := exec.LookPath(NPMBin()); err != nil {
		return fmt.Errorf("npm is not available in PATH (%s); install Node.js/npm on the host", NPMBin())
	}
	installCtx, cancel := context.WithTimeout(ctx, installTimeout())
	defer cancel()
	cmd := exec.CommandContext(installCtx, NPMBin(), "install", "--silent", "--no-audit", "--no-fund")
	cmd.Dir = runnerDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install @openai/codex-sdk runner dependency: %w: %s", err, strings.TrimSpace(stderr.String()))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
