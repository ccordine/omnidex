package hostbridge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultPickTimeout = 5 * time.Minute

// PickDirectory opens a native folder picker on the host and returns an absolute path.
func PickDirectory(ctx context.Context, startPath string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startPath = strings.TrimSpace(startPath)
	if startPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			startPath = home
		}
	}
	if startPath != "" {
		if abs, err := filepath.Abs(startPath); err == nil {
			if stat, err := os.Stat(abs); err == nil && stat.IsDir() {
				startPath = abs
			}
		}
	}

	type attempt struct {
		name string
		run  func(context.Context, string) (string, error)
	}
	attempts := []attempt{
		{name: "zenity", run: pickWithZenity},
		{name: "kdialog", run: pickWithKDialog},
	}
	if runtime.GOOS == "darwin" {
		attempts = append(attempts, attempt{name: "osascript", run: pickWithOsascript})
	}

	var failures []string
	for _, item := range attempts {
		pickCtx, cancel := context.WithTimeout(ctx, defaultPickTimeout)
		path, err := item.run(pickCtx, startPath)
		cancel()
		if err == nil && strings.TrimSpace(path) != "" {
			abs, absErr := filepath.Abs(strings.TrimSpace(path))
			if absErr != nil {
				return "", absErr
			}
			if stat, statErr := os.Stat(abs); statErr != nil || !stat.IsDir() {
				return "", fmt.Errorf("picker returned non-directory: %s", abs)
			}
			return filepath.Clean(abs), nil
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", item.name, err))
		}
	}
	if len(failures) == 0 {
		return "", fmt.Errorf("directory picker canceled")
	}
	return "", fmt.Errorf("native directory picker unavailable (%s)", strings.Join(failures, "; "))
}

func pickWithZenity(ctx context.Context, startPath string) (string, error) {
	if _, err := exec.LookPath("zenity"); err != nil {
		return "", err
	}
	args := []string{"--file-selection", "--directory", "--title=Choose project directory"}
	if startPath != "" {
		args = append(args, "--filename="+startPath+string(os.PathSeparator))
	}
	return runPickerCommand(ctx, "zenity", args...)
}

func pickWithKDialog(ctx context.Context, startPath string) (string, error) {
	if _, err := exec.LookPath("kdialog"); err != nil {
		return "", err
	}
	args := []string{"--getexistingdirectory", startPath, "--title", "Choose project directory"}
	return runPickerCommand(ctx, "kdialog", args...)
}

func pickWithOsascript(ctx context.Context, startPath string) (string, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return "", err
	}
	start := strings.ReplaceAll(startPath, `"`, `\"`)
	if start == "" {
		start = "~"
	}
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "Choose project directory" default location (POSIX file "%s"))`, start)
	return runPickerCommand(ctx, "osascript", "-e", script)
}

func runPickerCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", fmt.Errorf("canceled")
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
