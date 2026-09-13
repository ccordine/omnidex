package experiment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

type cliRun func(context.Context, []string, io.Reader, io.Writer, io.Writer) error

type Docker struct{ cli cliRun }

// NewDocker performs no discovery or daemon operation. The first experiment
// is the first consumer of the configured Docker CLI context.
func NewDocker() *Docker { return &Docker{cli: runDockerCLI} }

func runDockerCLI(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, "docker", argv...)
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	command.WaitDelay = time.Second
	return command.Run()
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	overflow bool
	cancel   context.CancelFunc
}

func (buffer *boundedBuffer) Bytes() []byte { return buffer.buffer.Bytes() }

func (buffer *boundedBuffer) Len() int { return buffer.buffer.Len() }

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	remaining := MaxStreamBytes - buffer.Len()
	if remaining > 0 {
		_, _ = buffer.buffer.Write(value[:min(remaining, len(value))])
	}
	if len(value) > remaining {
		buffer.overflow = true
		buffer.cancel()
	}
	return len(value), nil
}

func (docker *Docker) capture(ctx context.Context, argv []string, stdin io.Reader) ([]byte, []byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout, stderr := &boundedBuffer{cancel: cancel}, &boundedBuffer{cancel: cancel}
	err := docker.cli(ctx, argv, stdin, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		err = fmt.Errorf("Docker command output exceeded the %d-byte stream bound", MaxStreamBytes)
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func dockerOperationError(operation string, stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	if len(stderr) > 4000 {
		stderr = stderr[:4000]
	}
	return fmt.Errorf("Docker experiment %s: %w; stderr=%q", operation, err, stderr)
}
