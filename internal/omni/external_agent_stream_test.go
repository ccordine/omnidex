package omni

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

type protocolTestExternalAgentSession struct {
	events       []AgentEvent
	startErr     error
	cancelErr    error
	cleanupErr   error
	cancelCount  int
	cleanupCount int
}

func (s *protocolTestExternalAgentSession) Start(_ context.Context, _ ExternalAgentJob) (<-chan AgentEvent, error) {
	if s.startErr != nil {
		return nil, s.startErr
	}
	events := make(chan AgentEvent, len(s.events))
	for _, event := range s.events {
		events <- event
	}
	close(events)
	return events, nil
}

func (s *protocolTestExternalAgentSession) Interrupt(context.Context, HumanCorrection) error {
	return nil
}

func (s *protocolTestExternalAgentSession) Cancel(context.Context, string) error {
	s.cancelCount++
	return s.cancelErr
}

func (s *protocolTestExternalAgentSession) Pause(context.Context) error  { return nil }
func (s *protocolTestExternalAgentSession) Resume(context.Context) error { return nil }

func (s *protocolTestExternalAgentSession) Cleanup(context.Context) error {
	s.cleanupCount++
	return s.cleanupErr
}

func externalAgentProtocolJob() ExternalAgentJob {
	return ExternalAgentJob{SessionID: "session-1", Agent: "codex", Prompt: "bounded task", Workspace: "."}
}

func TestStreamExternalAgentSessionAcceptsOneExplicitCompletion(t *testing.T) {
	session := &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "codex", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "codex", Type: "message", Message: "implemented change"},
		{SessionID: "session-1", Agent: "codex", Type: "completed", Message: "verified change"},
	}}

	result, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "verified change" {
		t.Fatalf("summary=%q want explicit completion summary", result.Summary)
	}
	if session.cleanupCount != 1 {
		t.Fatalf("cleanup count=%d want 1", session.cleanupCount)
	}
}

func TestStreamExternalAgentSessionRejectsErrorEvenWhenFollowedByCompletion(t *testing.T) {
	session := &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "cursor", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "cursor", Type: "error", Message: "Cursor run failed"},
		{SessionID: "session-1", Agent: "cursor", Type: "completed", Message: "session ended"},
	}}

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "Cursor run failed") {
		t.Fatalf("error=%v want streamed failure", err)
	}
	if session.cleanupCount != 1 {
		t.Fatalf("cleanup count=%d want 1", session.cleanupCount)
	}
}

func TestStreamExternalAgentSessionRejectsMissingCompletion(t *testing.T) {
	session := &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "codex", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "codex", Type: "message", Message: "stream stopped"},
	}}

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "without a completed event") {
		t.Fatalf("error=%v want missing completion failure", err)
	}
}

func TestStreamExternalAgentSessionRejectsDuplicateCompletion(t *testing.T) {
	session := &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "codex", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "codex", Type: "completed", Message: "first"},
		{SessionID: "session-1", Agent: "codex", Type: "completed", Message: "second"},
	}}

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate completed event") {
		t.Fatalf("error=%v want duplicate completion failure", err)
	}
}

func TestStreamExternalAgentSessionCancelsWhenEventConsumerFails(t *testing.T) {
	session := &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "codex", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "codex", Type: "completed", Message: "done"},
	}}
	consumerErr := errors.New("persist streamed event")

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), func(AgentEvent) error {
		return consumerErr
	})
	if !errors.Is(err, consumerErr) {
		t.Fatalf("error=%v want consumer error", err)
	}
	if session.cancelCount != 1 || session.cleanupCount != 1 {
		t.Fatalf("cancel/cleanup=%d/%d want 1/1", session.cancelCount, session.cleanupCount)
	}
}

type failingStreamArchitectAgent struct {
	session *protocolTestExternalAgentSession
}

func (a *failingStreamArchitectAgent) RunArchitectTask(context.Context, CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	return CursorArchitectAgentResult{Summary: "non-stream fallback must not run"}, nil
}

func (a *failingStreamArchitectAgent) NewExternalAgentSession(CursorArchitectAgentInput) (ExternalAgentSession, error) {
	return a.session, nil
}

func TestRunExternalArchitectAgentTaskRejectsStreamFailure(t *testing.T) {
	agent := &failingStreamArchitectAgent{session: &protocolTestExternalAgentSession{events: []AgentEvent{
		{SessionID: "session-1", Agent: "codex", Type: "started", Message: "started"},
		{SessionID: "session-1", Agent: "codex", Type: "error", Message: "turn failed"},
	}}}

	_, err := runExternalArchitectAgentTask(t.Context(), agent, "codex_sdk", CursorArchitectAgentInput{Workspace: "."}, nil)
	if err == nil || !strings.Contains(err.Error(), "turn failed") {
		t.Fatalf("error=%v want stream failure", err)
	}
}

type failingNonStreamArchitectAgent struct{}

func (*failingNonStreamArchitectAgent) RunArchitectTask(context.Context, CursorArchitectAgentInput) (CursorArchitectAgentResult, error) {
	return CursorArchitectAgentResult{}, errors.New("external implementation failed")
}

func TestRunExternalArchitectAgentLaneDoesNotFallbackAfterSelectedAgentFailure(t *testing.T) {
	result := &CommandDecisionResult{}
	handled, err := runExternalArchitectAgentLane(
		t.Context(),
		1,
		"task",
		"task",
		ImplementationArchitectContract{
			TargetRoot:  ".",
			CurrentItem: &ArchitectWorkItem{ID: "implementation"},
		},
		structuredCommandDecisionRunConfig{CurrentWorkingDirectory: t.TempDir()},
		WorksiteSurvey{},
		nil,
		nil,
		nil,
		result,
		&failingNonStreamArchitectAgent{},
		"codex_sdk",
	)
	if !handled {
		t.Fatal("selected external-agent failure must remain handled and must not route to another implementation")
	}
	if err == nil || !strings.Contains(err.Error(), "external implementation failed") {
		t.Fatalf("error=%v want selected external-agent failure", err)
	}
}

func TestRestartExternalAgentSessionStopsWhenPreviousSessionCannotCleanUp(t *testing.T) {
	active := &protocolTestExternalAgentSession{
		cancelErr:  errors.New("cancel failed"),
		cleanupErr: errors.New("cleanup failed"),
	}
	provider := &failingStreamArchitectAgent{session: &protocolTestExternalAgentSession{}}

	events, revised, err := restartExternalAgentSessionWithCorrection(
		t.Context(),
		active,
		provider,
		"codex_sdk",
		CursorArchitectAgentInput{Workspace: "."},
		HumanCorrection{Message: "change direction"},
	)
	if err == nil || !strings.Contains(err.Error(), "stop external agent") {
		t.Fatalf("error=%v want explicit stop failure", err)
	}
	if events != nil {
		t.Fatalf("events=%v want no replacement session", events)
	}
	if revised.Workspace != "." {
		t.Fatalf("revised input changed after failed stop: %#v", revised)
	}
}

func TestCommandExternalAgentSessionDoesNotInventCompletion(t *testing.T) {
	session := &externalAgentCommandSession{
		agent: "codex",
		command: func(ctx context.Context, _ ExternalAgentJob) (*exec.Cmd, func() error, error) {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' '{"agent":"codex","type":"started","message":"started"}'`)
			return cmd, nil, nil
		},
	}

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "without a completed event") {
		t.Fatalf("error=%v want missing completion failure", err)
	}
}

func TestCommandExternalAgentSessionRejectsMalformedNDJSON(t *testing.T) {
	session := &externalAgentCommandSession{
		agent: "cursor",
		command: func(ctx context.Context, _ ExternalAgentJob) (*exec.Cmd, func() error, error) {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' '{not-json}'`)
			return cmd, nil, nil
		},
	}

	_, err := StreamExternalAgentSession(t.Context(), session, externalAgentProtocolJob(), nil)
	if err == nil || !strings.Contains(err.Error(), "decode external agent event") {
		t.Fatalf("error=%v want NDJSON protocol failure", err)
	}
}
