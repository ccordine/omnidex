package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const omniRuntimeDirEnv = "OMNIDEX_DIR"

func runBuild(args []string) {
	runScriptCommandOrExit("scripts/build-core.sh", args)
}

func runUpdate(args []string) {
	runScriptCommandOrExit("update.sh", resolveUpdateArgs(args))
}

func runUninstall(args []string) {
	runScriptCommandOrExit("uninstall.sh", args)
}

func resolveUpdateArgs(args []string) []string {
	if hasFlag(args, "--prefix") {
		return args
	}

	cwd := strings.TrimSpace(currentWorkingDirectory())
	if cwd == "" {
		return args
	}
	if !looksLikeOmnidexRepoRoot(cwd) {
		return args
	}

	out := make([]string, 0, len(args)+2)
	out = append(out, "--prefix", cwd)
	out = append(out, args...)
	return out
}

func hasFlag(args []string, longName string) bool {
	longName = strings.TrimSpace(longName)
	if longName == "" {
		return false
	}
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == longName || strings.HasPrefix(arg, longName+"=") {
			return true
		}
	}
	return false
}

func looksLikeOmnidexRepoRoot(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}

	requiredFiles := []string{"go.mod", "docker-compose.yml", "update.sh"}
	for _, name := range requiredFiles {
		if !scriptFileExists(filepath.Join(root, name)) {
			return false
		}
	}
	gitInfo, err := os.Stat(filepath.Join(root, ".git"))
	if err != nil || !gitInfo.IsDir() {
		return false
	}
	return true
}

func runScriptCommandOrExit(relativeScript string, args []string) {
	scriptPath, err := findManagedScriptPath(relativeScript)
	if err != nil {
		die(err.Error())
	}

	runArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("bash", runArgs...)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		die(fmt.Sprintf("failed to run %s: %v", relativeScript, err))
	}
}

func findManagedScriptPath(relativeScript string) (string, error) {
	relativeScript = filepath.Clean(strings.TrimSpace(relativeScript))
	if relativeScript == "" || relativeScript == "." {
		return "", fmt.Errorf("invalid script path")
	}
	if filepath.IsAbs(relativeScript) {
		if scriptFileExists(relativeScript) {
			return relativeScript, nil
		}
		return "", fmt.Errorf("script not found: %s", relativeScript)
	}

	roots := runtimeRootCandidates(
		strings.TrimSpace(os.Getenv(omniRuntimeDirEnv)),
		currentWorkingDirectory(),
		currentExecutablePath(),
	)

	if script := locateScriptUnderRoots(roots, relativeScript); script != "" {
		return script, nil
	}

	return "", fmt.Errorf(
		"unable to locate %s; run from the Omnidex repo root or set %s",
		relativeScript,
		omniRuntimeDirEnv,
	)
}

func runtimeRootCandidates(envRoot, cwd, executablePath string) []string {
	raw := []string{envRoot}
	if executablePath != "" {
		exeDir := filepath.Dir(executablePath)
		raw = append(raw, exeDir, filepath.Dir(exeDir))
	}
	raw = append(raw, cwd)
	return dedupeAbsolutePaths(raw)
}

func locateScriptUnderRoots(roots []string, relativeScript string) string {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if script := filepath.Join(root, relativeScript); scriptFileExists(script) {
			return script
		}
	}
	return ""
}

func dedupeAbsolutePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		clean := strings.TrimSpace(raw)
		if clean == "" {
			continue
		}
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
		clean = filepath.Clean(clean)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func scriptFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func currentWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cwd)
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil && strings.TrimSpace(resolved) != "" {
		path = resolved
	}
	return path
}
