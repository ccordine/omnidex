package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func runLocalSudoCommand(args []string, timeout time.Duration) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty sudo command")
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return "", errors.New("`sudo` is not available on this system")
	}
	if err := ensureSudoCredential(); err != nil {
		return "", err
	}
	sudoArgs := append([]string{"-n"}, args...)
	return runLocalCommand(append([]string{"sudo"}, sudoArgs...), timeout)
}

func ensureSudoCredential() error {
	if _, err := runLocalCommand([]string{"sudo", "-n", "true"}, 4*time.Second); err == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), localShellSudoAuthTimeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, "sudo", "-v")
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		cmd.Stdin = tty
		cmd.Stdout = tty
		cmd.Stderr = tty
	} else {
		if !isCharDevice(os.Stdin) || !isCharDevice(os.Stdout) {
			return errors.New("sudo authentication required but no interactive terminal is available (run `sudo -v` manually and retry)")
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.New("sudo authentication timed out")
		}
		return fmt.Errorf("sudo authentication failed: %w", err)
	}
	if _, err := runLocalCommand([]string{"sudo", "-n", "true"}, 4*time.Second); err != nil {
		return errors.New("sudo credentials are unavailable after authentication")
	}
	return nil
}

func commandExists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	_, err := exec.LookPath(name)
	return err == nil
}

func runLocalCommand(args []string, timeout time.Duration) (string, error) {
	return runLocalCommandMax(args, timeout, localShellOutputMaxChars)
}

func runLocalCommandMax(args []string, timeout time.Duration, maxChars int) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty command")
	}
	if maxChars <= 0 {
		maxChars = localShellOutputMaxChars
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := tracedExecCommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > maxChars {
		text = text[:maxChars] + "...(truncated)"
	}
	if ctx.Err() == context.DeadlineExceeded {
		if text == "" {
			text = args[0] + " timed out"
		}
		return "", errors.New(text)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", errors.New(text)
	}
	return text, nil
}

func validateRelativePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is required")
	}
	if strings.Contains(path, "~") {
		return errors.New("home-expansion (~) paths are not allowed in local chat mode")
	}
	if filepath.IsAbs(path) {
		return errors.New("absolute paths are not allowed in local chat mode")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return errors.New("path must reference a file")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("paths outside the current directory are not allowed in local chat mode")
	}
	return nil
}

func updateLocalShellStateFromAssistant(state *localShellState, assistantResult string) {
	if state == nil {
		return
	}
	command, ok := extractSuggestedSafeCommand(assistantResult)
	if !ok {
		return
	}
	state.LastSuggestedCommand = command
	state.LastSuggestedAt = time.Now()
}

func extractSuggestedSafeCommand(text string) (string, bool) {
	matches := shellBacktickPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			continue
		}
		if strings.ContainsAny(candidate, "|&;<>") {
			continue
		}
		args := strings.Fields(candidate)
		if len(args) == 0 {
			continue
		}
		if _, ok := allowedLocalShellCommands[strings.ToLower(args[0])]; !ok {
			continue
		}
		return strings.Join(args, " "), true
	}
	return "", false
}
