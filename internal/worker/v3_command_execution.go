package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type v3CommandExecution struct {
	Stdout          string
	Stderr          string
	StdoutBytes     int64
	StderrBytes     int64
	StdoutTruncated bool
	StderrTruncated bool
	ExitCode        int
	Duration        time.Duration
	RunError        error
	ContextError    error
}

func runValidatedV3Command(
	ctx context.Context,
	root string,
	command codeCommand,
) (v3CommandExecution, error) {
	var zero v3CommandExecution
	if ctx == nil {
		return zero, fmt.Errorf("command.run requires a context")
	}
	root = strings.TrimSpace(root)
	if err := validateV3CommandRoot(root); err != nil {
		return zero, err
	}
	program := strings.TrimSpace(command.Program)
	args := append([]string(nil), command.Args...)
	if err := validateV3CommandForProfile(program, args, command.Profile); err != nil {
		return zero, err
	}
	if err := validateV3CommandEnvironment(command); err != nil {
		return zero, err
	}
	executionRoot, commandEnvironment, err := resolveV3CommandExecution(ctx, root, program)
	if err != nil {
		return zero, err
	}
	timeout := command.Timeout
	if timeout == 0 {
		timeout = defaultV3CommandLimit
	}
	if timeout <= 0 || timeout > maxV3CommandLimit {
		return zero, fmt.Errorf(
			"command.run timeout must be between 1 and %d seconds",
			int(maxV3CommandLimit/time.Second),
		)
	}
	commandHome, err := os.MkdirTemp("", "omnidex-command-home-")
	if err != nil {
		return zero, fmt.Errorf("create isolated command home: %w", err)
	}
	defer os.RemoveAll(commandHome)
	environment, err := v3CommandEnvironment(commandHome)
	if err != nil {
		return zero, err
	}
	environment = append(environment, commandEnvironment...)
	if command.Profile == codeCommandProfileDeployment {
		environment = append(environment, "COMPOSE_DISABLE_ENV_FILE=1")
	}
	environment = append(environment, renderV3CommandEnvironment(command.Environment)...)
	stdout, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return zero, err
	}
	stderr, err := newBoundedCommandOutput(maxV3CommandOutput)
	if err != nil {
		return zero, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	process := exec.CommandContext(runCtx, program, args...)
	process.Dir = executionRoot
	process.Env = environment
	process.Stdout = stdout
	process.Stderr = stderr
	runErr := process.Run()
	result := v3CommandExecution{Duration: time.Since(started), RunError: runErr, ContextError: runCtx.Err()}
	result.Stdout, result.StdoutBytes, result.StdoutTruncated = stdout.Result()
	result.Stderr, result.StderrBytes, result.StderrTruncated = stderr.Result()
	if runErr != nil {
		result.ExitCode = -1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	return result, nil
}

func validateV3CommandEnvironment(command codeCommand) error {
	if len(command.Environment) == 0 {
		return nil
	}
	if command.Profile != codeCommandProfileDeployment {
		return fmt.Errorf("command.run environment is permitted only for persistent deployment")
	}
	allowed := map[string]struct{}{
		"APP_KEY": {}, "DATABASE_PASSWORD": {}, "HOST_BIND_ADDRESS": {},
		"HOST_HTTP_PORT": {}, "SERVICE_STATE_DB_PASSWORD": {},
	}
	for name, value := range command.Environment {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("persistent deployment environment name %q is not registered", name)
		}
		if value == "" || len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("persistent deployment environment value for %s is invalid", name)
		}
	}
	return nil
}

func renderV3CommandEnvironment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, name := range keys {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

func validateV3CommandRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) || root != filepath.Clean(root) {
		return fmt.Errorf("command.run requires one absolute server-authoritative root")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("command.run root is not an exact directory: %s", root)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return fmt.Errorf("command.run root contains a symbolic-link boundary: %s", root)
	}
	return nil
}

func v3CommandEnvironment(commandHome string) ([]string, error) {
	pathValue := strings.TrimSpace(os.Getenv("PATH"))
	if pathValue == "" {
		return nil, fmt.Errorf("command.run requires a non-empty server PATH")
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if directory == "" || !filepath.IsAbs(directory) || directory != filepath.Clean(directory) ||
			strings.ContainsAny(directory, "\x00\r\n") {
			return nil, fmt.Errorf("command.run server PATH must contain only exact absolute directories")
		}
	}
	info, err := os.Lstat(commandHome)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("command.run isolated home is not an exact directory")
	}
	return []string{
		"PATH=" + pathValue,
		"HOME=" + commandHome,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"TZ=UTC",
		"CI=1",
		"NO_COLOR=1",
		"PAGER=cat",
		"GIT_PAGER=cat",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GOTOOLCHAIN=local",
		"CARGO_NET_OFFLINE=true",
		"npm_config_ignore_scripts=true",
		"npm_config_audit=false",
		"npm_config_fund=false",
		"npm_config_update_notifier=false",
		"COMPOSER_NO_INTERACTION=1",
		"COMPOSER_DISABLE_NETWORK=1",
	}, nil
}

func renderV3CommandOutput(execution v3CommandExecution) string {
	parts := make([]string, 0, 2)
	if stdout := strings.TrimSpace(execution.Stdout); stdout != "" {
		parts = append(parts, stdout)
	}
	if stderr := strings.TrimSpace(execution.Stderr); stderr != "" {
		parts = append(parts, stderr)
	}
	return strings.Join(parts, "\n")
}
