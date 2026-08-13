package omni

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/hostbridge"
)

// UseHostBridgeExternalAgents reports whether Cursor/Codex should execute on the host
// machine via the bridge instead of inside the core process (e.g. Docker).
func UseHostBridgeExternalAgents() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL")), "true") {
		return false
	}
	return strings.TrimSpace(os.Getenv("HOST_AGENT_URL")) != ""
}

func hostBridgeClientFromEnv() *hostbridge.Client {
	url := strings.TrimSpace(os.Getenv("HOST_AGENT_URL"))
	if url == "" {
		return nil
	}
	token := strings.TrimSpace(os.Getenv("HOST_AGENT_TOKEN"))
	return hostbridge.NewClient(url, token, 2*time.Hour)
}

type hostBridgeExternalAgentSession struct {
	agent   string
	client  *hostbridge.Client
	apiKey  string
	model   string
	runtime ExternalAgentRuntimeOptions
}

type ExternalAgentRuntimeOptions struct {
	CodexPath       string
	ReasoningEffort string
	SandboxMode     string
	ApprovalPolicy  string
	NetworkAccess   string
	WebSearchMode   string
}

func newHostBridgeExternalAgentSession(agent, apiKey, model, codexPath string) (ExternalAgentSession, error) {
	return newHostBridgeExternalAgentSessionWithOptions(agent, apiKey, model, codexPath, ExternalAgentRuntimeOptions{})
}

func newHostBridgeExternalAgentSessionWithOptions(agent, apiKey, model, codexPath string, options ExternalAgentRuntimeOptions) (ExternalAgentSession, error) {
	client := hostBridgeClientFromEnv()
	if client == nil {
		return nil, fmt.Errorf("HOST_AGENT_URL is not configured; external agents must run on the host via `omni host serve` when core runs in Docker")
	}
	options.CodexPath = codexPath
	return &hostBridgeExternalAgentSession{
		agent:   agent,
		client:  client,
		apiKey:  apiKey,
		model:   model,
		runtime: options,
	}, nil
}

func (s *hostBridgeExternalAgentSession) Start(ctx context.Context, job ExternalAgentJob) (<-chan agentstream.Event, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("host bridge external agent session is not configured")
	}
	workspace := strings.TrimSpace(job.Workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required for host external agent execution")
	}

	body, err := s.client.RunExternalAgent(ctx, hostbridge.ExternalAgentRunRequest{
		Agent:           s.agent,
		APIKey:          s.apiKey,
		Model:           s.model,
		Workspace:       workspace,
		Prompt:          job.Prompt,
		CodexPath:       s.runtime.CodexPath,
		ReasoningEffort: s.runtime.ReasoningEffort,
		SandboxMode:     s.runtime.SandboxMode,
		ApprovalPolicy:  s.runtime.ApprovalPolicy,
		NetworkAccess:   s.runtime.NetworkAccess,
		WebSearchMode:   s.runtime.WebSearchMode,
	})
	if err != nil {
		return nil, err
	}

	events := make(chan agentstream.Event, 32)
	go func() {
		defer close(events)
		defer body.Close()
		readErr := hostbridge.ReadExternalAgentEvents(body, func(event agentstream.Event) error {
			if event.SessionID == "" {
				event.SessionID = job.SessionID
			}
			select {
			case events <- event:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if readErr != nil && ctx.Err() == nil {
			events <- agentstream.Event{
				SessionID: job.SessionID,
				Agent:     s.agent,
				Type:      agentstream.EventError,
				Message:   fmt.Sprintf("read host bridge external agent stream: %v", readErr),
			}
		}
	}()
	return events, nil
}

func (s *hostBridgeExternalAgentSession) Cancel(ctx context.Context, reason string) error {
	return fmt.Errorf("%s host bridge sessions cancel with the parent job context", s.agent)
}

func (s *hostBridgeExternalAgentSession) Cleanup(ctx context.Context) error {
	return nil
}
