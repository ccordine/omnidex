package omni

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type ExternalAgentJob struct {
	SessionID string                     `json:"session_id,omitempty"`
	Agent     string                     `json:"agent"`
	Mode      string                     `json:"mode"`
	Packet    CursorImplementationPacket `json:"packet"`
	Prompt    string                     `json:"prompt"`
	Workspace string                     `json:"workspace"`
}

type HumanCorrection struct {
	Message               string   `json:"message"`
	Authority             string   `json:"authority,omitempty"`
	ForbiddenDependencies []string `json:"forbidden_dependencies,omitempty"`
	AllowedFiles          []string `json:"allowed_files,omitempty"`
}

type AgentEvent struct {
	SessionID string          `json:"session_id,omitempty"`
	Agent     string          `json:"agent"`
	Type      string          `json:"type"`
	Message   string          `json:"message,omitempty"`
	Command   string          `json:"command,omitempty"`
	Files     []string        `json:"files,omitempty"`
	Evidence  []string        `json:"evidence,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

type ExternalAgentSession interface {
	Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error)
	Interrupt(ctx context.Context, correction HumanCorrection) error
	Cancel(ctx context.Context, reason string) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Cleanup(ctx context.Context) error
}

type externalAgentCommandSession struct {
	agent   string
	command func(context.Context, ExternalAgentJob) (*exec.Cmd, error)

	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func (s *externalAgentCommandSession) Start(ctx context.Context, job ExternalAgentJob) (<-chan AgentEvent, error) {
	if s == nil || s.command == nil {
		return nil, fmt.Errorf("external agent session is not configured")
	}
	runCtx, cancel := context.WithCancel(ctx)
	cmd, err := s.command(runCtx, job)
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	s.mu.Lock()
	s.cmd = cmd
	s.cancel = cancel
	s.mu.Unlock()

	events := make(chan AgentEvent, 32)
	go func() {
		defer close(events)
		defer cancel()
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					events <- AgentEvent{SessionID: job.SessionID, Agent: job.Agent, Type: "error", Message: line}
				}
			}
		}()
		scanExternalAgentJSONL(stdout, job, events)
		if err := cmd.Wait(); err != nil {
			events <- AgentEvent{SessionID: job.SessionID, Agent: job.Agent, Type: "error", Message: err.Error()}
		}
		<-stderrDone
		events <- AgentEvent{SessionID: job.SessionID, Agent: job.Agent, Type: "completed", Message: "external agent session ended"}
	}()
	return events, nil
}

func (s *externalAgentCommandSession) Interrupt(ctx context.Context, correction HumanCorrection) error {
	return fmt.Errorf("%s external agent interruption is not supported by this adapter; cancel and restart with a revised packet", s.agent)
}

func (s *externalAgentCommandSession) Cancel(ctx context.Context, reason string) error {
	s.mu.Lock()
	cancel := s.cancel
	cmd := s.cmd
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

func (s *externalAgentCommandSession) Pause(ctx context.Context) error {
	return fmt.Errorf("%s external agent pause is not supported by this adapter", s.agent)
}

func (s *externalAgentCommandSession) Resume(ctx context.Context) error {
	return fmt.Errorf("%s external agent resume is not supported by this adapter", s.agent)
}

func (s *externalAgentCommandSession) Cleanup(ctx context.Context) error {
	return s.Cancel(ctx, "cleanup")
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
		reqFile.Close()
		return "", fmt.Errorf("write external agent request: %w", err)
	}
	if err := reqFile.Close(); err != nil {
		return "", fmt.Errorf("close external agent request: %w", err)
	}
	return reqPath, nil
}

func resultFromExternalAgentEvents(events <-chan AgentEvent) CursorArchitectAgentResult {
	result := CursorArchitectAgentResult{}
	output := []string{}
	for event := range events {
		if (event.Type == "completed" || event.Type == "interrupted") && result.Summary == "" && !strings.Contains(event.Message, "session ended") {
			if event.Message != "" {
				result.Summary = event.Message
			}
		}
		if event.SessionID != "" {
			result.RunID = event.SessionID
		}
		if event.Agent != "" {
			result.AgentID = event.Agent
		}
		if event.Message != "" {
			output = append(output, event.Message)
		} else if len(event.Raw) > 0 {
			output = append(output, string(event.Raw))
		}
	}
	result.Output = strings.Join(output, "\n")
	if result.Summary == "" {
		result.Summary = result.Output
	}
	return result
}

func scanExternalAgentJSONL(stdout interface{ Read([]byte) (int, error) }, job ExternalAgentJob, events chan<- AgentEvent) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event AgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			events <- AgentEvent{SessionID: job.SessionID, Agent: job.Agent, Type: "message", Message: line}
			continue
		}
		if event.SessionID == "" {
			event.SessionID = job.SessionID
		}
		if event.Agent == "" {
			event.Agent = job.Agent
		}
		events <- event
	}
}

func structuredEventFromExternalAgentEvent(event AgentEvent) StructuredCommandEvent {
	details := map[string]string{
		"session_id": event.SessionID,
		"agent":      event.Agent,
		"type":       event.Type,
	}
	if event.Command != "" {
		details["command"] = truncateStructuredTimelineValue(event.Command)
	}
	if len(event.Files) > 0 {
		details["files"] = strings.Join(event.Files, ",")
	}
	return StructuredCommandEvent{
		Type:    "external_agent_" + strings.ReplaceAll(strings.TrimSpace(event.Type), ".", "_"),
		Summary: firstNonEmpty(event.Message, "External agent event"),
		Details: details,
	}
}

func applyHumanCorrectionToExternalAgentInput(input CursorArchitectAgentInput, correction HumanCorrection) CursorArchitectAgentInput {
	message := strings.TrimSpace(correction.Message)
	if strings.TrimSpace(correction.Authority) == "" {
		correction.Authority = "user"
	}
	if message != "" {
		input.Packet.PreparedContext = appendUniqueStrings(input.Packet.PreparedContext, fmt.Sprintf("human_correction[%s]: %s", strings.TrimSpace(correction.Authority), message))
	}
	for _, dep := range correction.ForbiddenDependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		input.Packet.Forbidden = appendUniqueStrings(input.Packet.Forbidden, "do not add dependency: "+dep)
	}
	if len(correction.AllowedFiles) > 0 {
		allowed := make([]string, 0, len(correction.AllowedFiles))
		for _, path := range correction.AllowedFiles {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				allowed = append(allowed, filepath.ToSlash(trimmed))
			}
		}
		if len(allowed) > 0 {
			input.Packet.EditSurface = appendUniqueStrings(nil, allowed...)
		}
	}
	return input
}

func restartExternalAgentSessionWithCorrection(ctx context.Context, active ExternalAgentSession, provider ExternalAgentSessionProvider, agentName string, input CursorArchitectAgentInput, correction HumanCorrection) (<-chan AgentEvent, CursorArchitectAgentInput, error) {
	if active != nil {
		_ = active.Cancel(ctx, "human correction invalidated active external-agent plan")
		_ = active.Cleanup(ctx)
	}
	revised := applyHumanCorrectionToExternalAgentInput(input, correction)
	session, err := provider.NewExternalAgentSession(revised)
	if err != nil {
		return nil, revised, err
	}
	job := ExternalAgentJob{
		SessionID: agentName + "-architect-corrected",
		Agent:     strings.TrimSuffix(agentName, "_sdk"),
		Mode:      "implementation",
		Packet:    revised.Packet,
		Prompt:    externalAgentPromptForName(agentName, revised),
		Workspace: revised.Workspace,
	}
	events, err := session.Start(ctx, job)
	if err != nil {
		return nil, revised, err
	}
	return events, revised, nil
}
