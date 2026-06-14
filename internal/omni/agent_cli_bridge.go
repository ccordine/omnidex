package omni

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (a *App) runAgentMode(args []string) error {
	bin, err := resolveAgentCLIBinary()
	if err != nil {
		return err
	}
	cliArgs := agentModeArgs(args)
	cmd := exec.Command(bin, cliArgs...)
	cmd.Stdin = a.in
	cmd.Stdout = a.out
	cmd.Stderr = a.errOut
	cmd.Env = append(os.Environ(), "OMNI_INVOKE_CWD="+workspacePathOrCurrentDir())
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
	if clean[0] == "chat" || isAgentCLIPassthroughCommand(clean[0]) {
		return clean
	}
	return append([]string{"chat"}, clean...)
}

func isAgentCLIPassthroughCommand(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "enqueue", "list", "show", "watch", "interrupt", "cancel", "replan", "continue", "feedback", "status", "metrics", "core:status", "queue:status", "ollama:status", "web:status", "config", "host":
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
