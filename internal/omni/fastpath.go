package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FastPathResult struct {
	Action    string            `json:"action"`
	Workspace string            `json:"workspace"`
	Success   bool              `json:"success"`
	Evidence  map[string]string `json:"evidence,omitempty"`
	Error     string            `json:"error,omitempty"`
}

func RunFastPath(ctx context.Context, action, workspace string) FastPathResult {
	action = strings.TrimSpace(action)
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = workspacePathOrCurrentDir()
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return FastPathResult{Action: action, Workspace: workspace, Error: err.Error()}
	}
	result := FastPathResult{Action: action, Workspace: abs, Evidence: map[string]string{}}
	switch action {
	case "git.branch":
		return runFastPathCommand(ctx, result, "git", "branch", "--show-current")
	case "git.status":
		return runFastPathCommand(ctx, result, "git", "status", "--short")
	case "git.diffstat":
		return runFastPathCommand(ctx, result, "git", "diff", "--stat")
	case "package.manager":
		result.Evidence["package_manager"] = detectPackageManager(abs)
		result.Success = true
		return result
	case "project.probe":
		probe := deterministicProjectProbe(abs)
		blob, err := json.Marshal(probe)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.Evidence["project_probe"] = string(blob)
		result.Success = true
		return result
	default:
		result.Error = fmt.Sprintf("unknown fast-path action %q", action)
		return result
	}
}

func runFastPathCommand(ctx context.Context, result FastPathResult, name string, args ...string) FastPathResult {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = result.Workspace
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.Evidence["command"] = strings.Join(append([]string{name}, args...), " ")
	result.Evidence["stdout"] = strings.TrimSpace(stdout.String())
	result.Evidence["stderr"] = strings.TrimSpace(stderr.String())
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Success = true
	return result
}

func detectPackageManager(workspace string) string {
	switch {
	case fileExists(filepath.Join(workspace, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(workspace, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(workspace, "package-lock.json")):
		return "npm"
	case fileExists(filepath.Join(workspace, "package.json")):
		return "npm"
	case fileExists(filepath.Join(workspace, "go.mod")):
		return "go"
	case fileExists(filepath.Join(workspace, "Cargo.toml")):
		return "cargo"
	case fileExists(filepath.Join(workspace, "pyproject.toml")):
		return "python"
	default:
		return "unknown"
	}
}

type DeterministicProjectProbe struct {
	PackageManager string   `json:"package_manager"`
	TestCommands   []string `json:"test_commands,omitempty"`
	BuildCommands  []string `json:"build_commands,omitempty"`
	RunCommands    []string `json:"run_commands,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

func deterministicProjectProbe(workspace string) DeterministicProjectProbe {
	probe := DeterministicProjectProbe{PackageManager: detectPackageManager(workspace)}
	if fileExists(filepath.Join(workspace, "go.mod")) {
		probe.TestCommands = append(probe.TestCommands, "go test ./...")
		probe.BuildCommands = append(probe.BuildCommands, "go build ./...")
		probe.Evidence = append(probe.Evidence, "go.mod exists")
	}
	if fileExists(filepath.Join(workspace, "package.json")) {
		probe.Evidence = append(probe.Evidence, "package.json exists")
		if scripts := packageJSONScripts(workspace); len(scripts) > 0 {
			if _, ok := scripts["test"]; ok {
				probe.TestCommands = append(probe.TestCommands, "npm test")
			}
			if _, ok := scripts["build"]; ok {
				probe.BuildCommands = append(probe.BuildCommands, "npm run build")
			}
			if _, ok := scripts["dev"]; ok {
				probe.RunCommands = append(probe.RunCommands, "npm run dev")
			}
			if _, ok := scripts["start"]; ok {
				probe.RunCommands = append(probe.RunCommands, "npm start")
			}
		}
	}
	return probe
}

func packageJSONScripts(workspace string) map[string]string {
	blob, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		return nil
	}
	var payload struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(blob, &payload); err != nil {
		return nil
	}
	return payload.Scripts
}

func formatFastPathResult(result FastPathResult) string {
	lines := []string{
		"action=" + result.Action,
		"workspace=" + result.Workspace,
		fmt.Sprintf("success=%t", result.Success),
	}
	if result.Error != "" {
		lines = append(lines, "error="+result.Error)
	}
	keys := make([]string, 0, len(result.Evidence))
	for key := range result.Evidence {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(result.Evidence[key]); value != "" {
			lines = append(lines, key+"="+value)
		}
	}
	return strings.Join(lines, "\n")
}
