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

const RunnerScript = `import { Agent, CursorAgentError } from "@cursor/sdk";
import fs from "node:fs/promises";

function emit(event) {
  console.log(JSON.stringify(event));
}

function formatError(err) {
  if (err == null) {
    return "unknown error (check CURSOR_API_KEY and host PATH includes node, npm, base64, and cursor CLI)";
  }
  const parts = [];
  const push = (value) => {
    const text = String(value ?? "").trim();
    if (text && text !== "Error" && text !== "undefined" && text !== "null") {
      parts.push(text);
    }
  };
  push(err.message);
  if (err.cause) {
    push(err.cause.message);
    push(err.cause.code);
  }
  push(err.code);
  push(err.rawMessage);
  if (parts.length === 0) {
    return "unknown error (verify CURSOR_API_KEY in Admin → API secrets and host PATH for node/npm/base64)";
  }
  return parts.join("; ");
}

async function disposeAgent(agent) {
  if (!agent) return;
  if (typeof agent[Symbol.asyncDispose] === "function") {
    await agent[Symbol.asyncDispose]();
    return;
  }
  if (typeof agent.close === "function") {
    await agent.close();
  }
}

const requestPath = process.argv[2];
if (!requestPath) {
  throw new Error("request path is required");
}

const request = JSON.parse(await fs.readFile(requestPath, "utf8"));
let agent;
try {
  agent = await Agent.create({
    apiKey: request.api_key,
    model: { id: request.model || "composer-2.5" },
    local: {
      cwd: request.workspace || process.cwd(),
      settingSources: [],
    },
  });
} catch (err) {
  const message = "Cursor agent failed to launch: " + formatError(err);
  emit({ agent: "cursor", type: "error", message });
  console.error(message);
  process.exit(err instanceof CursorAgentError ? 1 : 2);
}

try {
  const run = await agent.send(request.prompt);
  const events = [];
  let lastErrorDetail = "";

  emit({ agent: "cursor", type: "started", message: "Cursor external implementation session started" });

  try {
    for await (const event of run.stream()) {
      events.push(event);
      if (event && typeof event === "object" && event.type === "status") {
        const status = String(event.status || "").toUpperCase();
        emit({ agent: "cursor", type: "status", message: JSON.stringify(event), raw: event });
        if (status === "ERROR") {
          lastErrorDetail = JSON.stringify(event);
        }
        continue;
      }
      const text = typeof event === "string" ? event : JSON.stringify(event);
      emit({ agent: "cursor", type: "message", message: text, raw: event });
    }
  } catch (err) {
    const message = "Cursor agent stream failed: " + formatError(err);
    emit({ agent: "cursor", type: "error", message });
    console.error(message);
    process.exit(2);
  }

  const result = await run.wait();
  const runStatus = String(result?.status || "").toLowerCase();
  if (runStatus === "error" || runStatus === "failed" || runStatus === "cancelled") {
    const detail =
      result?.error?.message ||
      result?.error ||
      result?.summary ||
      lastErrorDetail ||
      "Cursor run ended with status " + (result?.status || "error");
    emit({
      agent: "cursor",
      type: "error",
      message: String(detail),
      raw: { status: result?.status, run_id: result?.id, error: result?.error },
    });
    console.error(String(detail));
    process.exit(2);
  }

  let summary = "";
  if (result?.result != null) {
    summary = typeof result.result === "string" ? result.result : JSON.stringify(result.result);
  }

  emit({
    agent: "cursor",
    type: "completed",
    message: summary || "Cursor external implementation session completed",
    evidence: events.map((event) => (typeof event === "string" ? event : JSON.stringify(event))),
    raw: {
      agent_id: agent?.id || "",
      run_id: result?.id || "",
      status: result?.status || "completed",
    },
  });
} catch (err) {
  const message =
    err instanceof CursorAgentError
      ? "Cursor startup failed: " + formatError(err) + " (retryable=" + Boolean(err.isRetryable) + ")"
      : "Cursor agent failed: " + formatError(err);
  emit({ agent: "cursor", type: "error", message });
  console.error(message);
  process.exit(err instanceof CursorAgentError ? 1 : 2);
} finally {
  await disposeAgent(agent);
}
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
	cmd.Env = CommandEnv()
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
	cmd.Env = CommandEnv()
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
