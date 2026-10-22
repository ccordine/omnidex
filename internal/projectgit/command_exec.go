package projectgit

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
)

const (
	maxGitCommandOutputBytes = 4 * 1024 * 1024
	maxGitCommandErrorBytes  = 4 * 1024
)

type execCommandRunner struct{}

type commandError struct {
	ExitCode int
	Output   string
	Cause    error
}

func (err *commandError) Error() string {
	if err.Output != "" {
		return err.Output
	}
	return err.Cause.Error()
}

func (err *commandError) Unwrap() error { return err.Cause }

func (execCommandRunner) Output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	output := &boundedCommandBuffer{limit: maxGitCommandOutputBytes}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	value := output.String()
	if err == nil {
		return value, nil
	}
	exitCode := -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		exitCode = exitError.ExitCode()
	}
	message := strings.TrimSpace(value)
	if len(message) > maxGitCommandErrorBytes {
		message = message[:maxGitCommandErrorBytes] + "…"
	}
	return "", &commandError{ExitCode: exitCode, Output: message, Cause: err}
}

type boundedCommandBuffer struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	exceeded bool
}

func (buffer *boundedCommandBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	buffer.data = append(buffer.data, value...)
	return len(value), nil
}

func (buffer *boundedCommandBuffer) Exceeded() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.exceeded
}

func (buffer *boundedCommandBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(buffer.data)
}
