package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const defaultConfigFileName = ".env"

func runConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	filePathFlag := fs.String("file", "", "config file path override")
	editorFlag := fs.String("editor", "", "editor command override (default: vim)")
	printOnly := fs.Bool("print", false, "print resolved config path and exit")
	_ = fs.Parse(args)

	configPath, err := resolveManagedConfigPath(strings.TrimSpace(*filePathFlag))
	if err != nil {
		die(err.Error())
	}
	if *printOnly {
		fmt.Println(configPath)
		return
	}

	editor := resolveConfigEditor(strings.TrimSpace(*editorFlag))
	invocation, err := editorCommandArgs(editor, configPath)
	if err != nil {
		die(err.Error())
	}

	if _, err := exec.LookPath(invocation[0]); err != nil {
		die(fmt.Sprintf("editor command not found: %s", invocation[0]))
	}

	cmd := exec.Command(invocation[0], invocation[1:]...)
	cmd.Dir = filepath.Dir(configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		die(fmt.Sprintf("failed to launch editor: %v", err))
	}
}

func resolveConfigEditor(explicit string) string {
	if clean := strings.TrimSpace(explicit); clean != "" {
		return clean
	}
	if clean := strings.TrimSpace(os.Getenv("OMNI_CONFIG_EDITOR")); clean != "" {
		return clean
	}
	if clean := strings.TrimSpace(os.Getenv("EDITOR")); clean != "" {
		return clean
	}
	return "vim"
}

func resolveManagedConfigPath(explicitPath string) (string, error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath != "" {
		return ensureExplicitConfigPath(explicitPath)
	}

	root, err := resolveManagedConfigRoot()
	if err != nil {
		return "", err
	}
	return ensureManagedConfigFile(root)
}

func ensureExplicitConfigPath(rawPath string) (string, error) {
	path := expandHomePath(rawPath)
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	path = filepath.Clean(path)

	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("config path points to a directory: %s", path)
		}
		return path, nil
	}

	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		parent = currentWorkingDirectory()
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", fmt.Errorf("config parent directory does not exist: %s", parent)
	}

	if err := writeDefaultConfigFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func resolveManagedConfigRoot() (string, error) {
	roots := runtimeRootCandidates(
		strings.TrimSpace(os.Getenv(omniRuntimeDirEnv)),
		currentWorkingDirectory(),
		currentExecutablePath(),
	)

	for _, root := range roots {
		if root == "" {
			continue
		}
		if scriptFileExists(filepath.Join(root, defaultConfigFileName)) {
			return root, nil
		}
	}

	for _, root := range roots {
		if root == "" {
			continue
		}
		if scriptFileExists(filepath.Join(root, "default.env")) ||
			scriptFileExists(filepath.Join(root, ".env.example")) ||
			scriptFileExists(filepath.Join(root, "go.mod")) ||
			scriptFileExists(filepath.Join(root, "docker-compose.yml")) {
			return root, nil
		}
	}

	return "", errors.New("unable to locate Omnidex config root; set OMNIDEX_DIR or pass --file")
}

func ensureManagedConfigFile(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("config root is required")
	}
	configPath := filepath.Join(root, defaultConfigFileName)
	if scriptFileExists(configPath) {
		return configPath, nil
	}

	for _, template := range []string{
		filepath.Join(root, "default.env"),
		filepath.Join(root, ".env.example"),
	} {
		if !scriptFileExists(template) {
			continue
		}
		if err := copyFileContents(template, configPath); err != nil {
			return "", err
		}
		return configPath, nil
	}

	if err := writeDefaultConfigFile(configPath); err != nil {
		return "", err
	}
	return configPath, nil
}

func copyFileContents(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0o644)
}

func writeDefaultConfigFile(path string) error {
	content := strings.Join([]string{
		"# Omnidex runtime configuration",
		"# Example keys:",
		"# LLM_PROVIDER=ollama",
		"# OLLAMA_MODEL=llama3.2",
		"# OPENAI_API_KEY=sk-...",
		"# OPENAI_MODEL=gpt-4.1-mini",
		"# OPENAI_EMBEDDING_MODEL=text-embedding-3-small",
		"# OLLAMA_MODEL_REASONING=llama3.2",
		"# OLLAMA_MODEL_PLANNER=llama3.2",
		"# OLLAMA_MODEL_ANALYZER=llama3.2",
		"# OLLAMA_MODEL_RESPONDER=llama3.2",
		"# OLLAMA_MODEL_SPECIALIST_PLANNER=llama3.2",
		"# OLLAMA_MODEL_SPECIALIST_REVIEW_VERIFICATION=llama3.2",
		"",
	}, "\n")
	return os.WriteFile(path, []byte(content), 0o644)
}

func editorCommandArgs(editor string, configPath string) ([]string, error) {
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return nil, errors.New("editor command is required")
	}
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return nil, errors.New("config path is required")
	}
	args := strings.Fields(editor)
	if len(args) == 0 {
		return nil, errors.New("editor command is required")
	}
	args = append(args, configPath)
	return args, nil
}
