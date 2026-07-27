package omni

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const externalAgentMaxEventBytes = 4 << 20

type externalAgentCommandFactory func(context.Context, ExternalAgentJob) (*exec.Cmd, func() error, error)

type externalAgentCommandSession struct {
	agent   string
	command externalAgentCommandFactory

	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	cleanup func() error
}

func (s *externalAgentCommandSession) Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error) {
	if s == nil || s.command == nil {
		return nil, fmt.Errorf("external agent session is not configured")
	}
	runCtx, cancel := context.WithCancel(ctx)
	cmd, cleanup, err := s.command(runCtx, job)
	if err != nil {
		cancel()
		return nil, err
	}
	if cmd == nil {
		cancel()
		return nil, errors.Join(fmt.Errorf("external agent command factory returned nil command"), runExternalAgentCleanup(cleanup))
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, errors.Join(fmt.Errorf("open external agent stdout: %w", err), runExternalAgentCleanup(cleanup))
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, errors.Join(fmt.Errorf("open external agent stderr: %w", err), runExternalAgentCleanup(cleanup))
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, errors.Join(fmt.Errorf("start external agent command: %w", err), runExternalAgentCleanup(cleanup))
	}
	s.mu.Lock()
	s.cmd = cmd
	s.cancel = cancel
	s.cleanup = cleanup
	s.mu.Unlock()

	events := make(chan AgentEvent, 32)
	go s.collectCommandEvents(runCtx, cancel, job, cmd, stdout, stderr, events)
	return events, nil
}

func (s *externalAgentCommandSession) collectCommandEvents(
	ctx context.Context,
	cancel context.CancelFunc,
	job ExternalAgentJob,
	cmd *exec.Cmd,
	stdout io.Reader,
	stderr io.Reader,
	events chan<- AgentEvent,
) {
	defer close(events)
	defer cancel()

	stderrDone := make(chan error, 1)
	go func() {
		stderrDone <- scanExternalAgentStderr(ctx, stderr, job, events)
	}()

	stdoutErr := scanExternalAgentJSONL(ctx, stdout, job, events)
	waitErr := cmd.Wait()
	stderrErr := <-stderrDone
	for _, streamErr := range []error{stdoutErr, stderrErr, waitErr} {
		if streamErr == nil {
			continue
		}
		emitAgentEvent(ctx, events, AgentEvent{
			SessionID: job.SessionID,
			Agent:     job.Agent,
			Type:      AgentEventError,
			Message:   streamErr.Error(),
		})
	}
}

func (s *externalAgentCommandSession) Interrupt(context.Context, HumanCorrection) error {
	return fmt.Errorf("%s external agent interruption is not supported by this adapter; cancel and restart with a revised packet", s.agent)
}

func (s *externalAgentCommandSession) Cancel(context.Context, string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	cmd := s.cmd
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill %s external agent process: %w", s.agent, err)
	}
	return nil
}

func (s *externalAgentCommandSession) Pause(context.Context) error {
	return fmt.Errorf("%s external agent pause is not supported by this adapter", s.agent)
}

func (s *externalAgentCommandSession) Resume(context.Context) error {
	return fmt.Errorf("%s external agent resume is not supported by this adapter", s.agent)
}

func (s *externalAgentCommandSession) Cleanup(ctx context.Context) error {
	if s == nil {
		return nil
	}
	cancelErr := s.Cancel(ctx, "cleanup")
	s.mu.Lock()
	cleanup := s.cleanup
	s.cleanup = nil
	s.mu.Unlock()
	return errors.Join(cancelErr, runExternalAgentCleanup(cleanup))
}

func scanExternalAgentJSONL(ctx context.Context, stdout io.Reader, job ExternalAgentJob, events chan<- AgentEvent) error {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), externalAgentMaxEventBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event AgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			if !emitAgentEvent(ctx, events, AgentEvent{
				SessionID: job.SessionID,
				Agent:     job.Agent,
				Type:      AgentEventError,
				Message:   fmt.Sprintf("decode external agent event: %v", err),
			}) {
				return ctx.Err()
			}
			continue
		}
		if event.SessionID == "" {
			event.SessionID = job.SessionID
		}
		if event.Agent == "" {
			event.Agent = job.Agent
		}
		if !emitAgentEvent(ctx, events, event) {
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read external agent stdout: %w", err)
	}
	return nil
}

func scanExternalAgentStderr(ctx context.Context, stderr io.Reader, job ExternalAgentJob, events chan<- AgentEvent) error {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), externalAgentMaxEventBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !emitAgentEvent(ctx, events, AgentEvent{
			SessionID: job.SessionID,
			Agent:     job.Agent,
			Type:      AgentEventError,
			Message:   line,
		}) {
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read external agent stderr: %w", err)
	}
	return nil
}

func emitAgentEvent(ctx context.Context, events chan<- AgentEvent, event AgentEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func writeExternalAgentRequest(pattern string, request any) (string, error) {
	blob, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal external agent request: %w", err)
	}
	reqFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create external agent request: %w", err)
	}
	reqPath := reqFile.Name()
	if _, err := reqFile.Write(blob); err != nil {
		closeErr := reqFile.Close()
		removeErr := os.Remove(reqPath)
		return "", errors.Join(fmt.Errorf("write external agent request: %w", err), closeErr, removeErr)
	}
	if err := reqFile.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("close external agent request: %w", err), os.Remove(reqPath))
	}
	return reqPath, nil
}

func removeExternalAgentRequest(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove external agent request %q: %w", path, err)
	}
	return nil
}

func runExternalAgentCleanup(cleanup func() error) error {
	if cleanup == nil {
		return nil
	}
	return cleanup()
}
