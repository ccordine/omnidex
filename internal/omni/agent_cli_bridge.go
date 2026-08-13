package omni

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (a *App) runAgentMode(args []string) error {
	return a.runAgentCLI(agentModeArgs(args), a.in)
}

func (a *App) runAgentCLI(args []string, input io.Reader) error {
	if a == nil || a.agentCLI == nil {
		return fmt.Errorf("agent-cli runner is not configured")
	}
	return a.agentCLI(args, input)
}

func (a *App) executeAgentCLI(args []string, input io.Reader) error {
	workspace, err := currentWorkspacePath()
	if err != nil {
		return err
	}
	bin, err := resolveAgentCLIBinary()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = input
	cmd.Stdout = a.out
	cmd.Stderr = a.errOut
	cmd.Env = append(os.Environ(), "OMNI_INVOKE_CWD="+workspace)
	return cmd.Run()
}

func agentModeArgs(args []string) []string {
	clean := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			clean = append(clean, arg)
		}
	}
	if len(clean) == 0 || strings.HasPrefix(clean[0], "-") {
		return append([]string{"chat"}, clean...)
	}
	if clean[0] == "chat" || clean[0] == "run" || isAgentCLIPassthroughCommand(clean[0]) {
		return clean
	}
	return append([]string{"chat"}, clean...)
}

func isAgentCLIPassthroughCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "list", "show", "watch", "interrupt", "cancel", "replan", "feedback", "status", "metrics", "core:status", "queue:status", "ollama:status", "ollama:prewarm", "web:status", "config", "host":
		return true
	default:
		return false
	}
}

func resolveAgentCLIBinary() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("OMNI_AGENT_CLI_BIN")); explicit != "" {
		if path, err := exec.LookPath(explicit); err == nil {
			return path, nil
		}
		if filepath.IsAbs(explicit) {
			return "", fmt.Errorf("OMNI_AGENT_CLI_BIN=%q is not executable", explicit)
		}
		return "", fmt.Errorf("OMNI_AGENT_CLI_BIN=%q was not found in PATH", explicit)
	}
	for _, candidate := range siblingAgentCLIBinaries() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	for _, name := range []string{"agent-cli", "acli"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("agent-cli binary not found; run `omni update --host-only` or install Omnidex again")
}

func siblingAgentCLIBinaries() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	names := []string{"agent-cli", "acli"}
	if runtime.GOOS == "windows" {
		names = []string{"agent-cli.exe", "acli.exe"}
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}
