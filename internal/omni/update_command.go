package omni

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (a *App) runUpdate(args []string) error {
	scriptPath, err := findManagedUpdateScript()
	if err != nil {
		return err
	}
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Stdin = a.in
	cmd.Stdout = a.out
	cmd.Stderr = a.errOut
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", scriptPath, err)
	}
	return nil
}

func findManagedUpdateScript() (string, error) {
	workspace, err := currentWorkspacePath()
	if err != nil {
		return "", err
	}
	roots := managedScriptRootCandidates(
		strings.TrimSpace(os.Getenv("OMNIDEX_DIR")),
		workspace,
		currentExecutablePath(),
	)
	if script := locateManagedScript(roots, "update.sh"); script != "" {
		return script, nil
	}
	return "", fmt.Errorf("unable to locate update.sh; run from the Omnidex install root or set OMNIDEX_DIR")
}

func managedScriptRootCandidates(envRoot, cwd, executablePath string) []string {
	roots := []string{envRoot}
	if executablePath = strings.TrimSpace(executablePath); executablePath != "" {
		executableDir := filepath.Dir(executablePath)
		roots = append(roots, executableDir, filepath.Dir(executableDir))
	}
	return dedupeCleanAbsPaths(append(roots, cwd))
}

func locateManagedScript(roots []string, scriptName string) string {
	scriptName = filepath.Clean(strings.TrimSpace(scriptName))
	if scriptName == "" || scriptName == "." || filepath.IsAbs(scriptName) {
		return ""
	}
	for _, root := range roots {
		candidate := filepath.Join(root, scriptName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return strings.TrimSpace(path)
}

func dedupeCleanAbsPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}
