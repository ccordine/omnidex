package codex

import (
	"context"
	"fmt"
	"sync"
)

// Bound native ephemeral-thread retention even during a long service lifetime.
const maxParserProcessCalls = 64

// parserProcess owns one tool-free transport. The gate bounds inference to one
// request; each request still starts its own ephemeral native thread.
type parserProcess struct {
	lifetime   context.Context
	cancel     context.CancelFunc
	calls      int
	gate       chan struct{}
	connection *connection
	cleanup    func()
	closed     bool
}

func NewParser(lifetime context.Context, command, directory string) (*Client, error) {
	if lifetime == nil {
		return nil, fmt.Errorf("parser requires a lifecycle context")
	}
	client, err := New(command, directory)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(lifetime)
	client.parser = &parserProcess{lifetime: lifetime, cancel: cancel, gate: make(chan struct{}, 1)}
	context.AfterFunc(lifetime, func() { _ = client.Close() })
	return client, nil
}

// Close waits for any canceled active call to release the transport, then reaps
// the process and removes only its private instruction/cwd scratch directory.
func (client *Client) Close() error {
	if client.parser == nil {
		return nil
	}
	p := client.parser
	p.cancel()
	p.gate <- struct{}{}
	defer func() { <-p.gate }()
	p.closed = true
	p.reset()
	return nil
}

func (p *parserProcess) reset() {
	if p.connection != nil {
		p.connection.close()
		p.connection = nil
	}
	p.calls = 0
	if p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil
	}
}

func (client *Client) generationConnection(ctx context.Context) (*connection, func(bool), error) {
	if client.parser == nil {
		connection, err := startConnection(ctx, client.command, client.directory)
		return connection, func(bool) {
			if connection != nil {
				connection.close()
			}
		}, err
	}
	p := client.parser
	select {
	case p.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	release := func() { <-p.gate }
	if p.closed || p.lifetime.Err() != nil || ctx.Err() != nil {
		release()
		return nil, nil, fmt.Errorf("parser process is closed or request canceled")
	}
	if p.connection == nil {
		scratch, cleanup, err := client.agentScratch()
		if err != nil {
			release()
			return nil, nil, err
		}
		connection, err := startIsolatedParserConnection(p.lifetime, client.command, scratch, scratch, parserInstructionOverrides())
		if err != nil {
			cleanup()
			release()
			return nil, nil, err
		}
		connection.isolatedParser = true
		p.connection, p.cleanup = connection, cleanup
	}
	connection := p.connection
	var once sync.Once
	return connection, func(healthy bool) {
		once.Do(func() {
			// Canceled calls must never leave unread frames for another question.
			p.calls++
			if !healthy || ctx.Err() != nil || p.lifetime.Err() != nil || p.calls >= maxParserProcessCalls {
				p.reset()
			}
			release()
		})
	}, nil
}

// These provider capabilities are separate from the older shell/apps/plugin
// switches in the shared boundary. Disable them only for the user parser.
func parserInstructionOverrides() []string {
	arguments := []string{"-c", "features.skip_host_skill_discovery=true", "-c", `developer_instructions=""`}
	// Capability flags do not suppress the provider's automatic context blocks.
	for _, key := range []string{"include_apps_instructions", "include_collaboration_mode_instructions", "include_environment_context", "include_environment_context_time", "include_permissions_instructions", "skills.include_instructions", "skills.bundled.enabled", "tools.experimental_request_user_input.enabled"} {
		arguments = append(arguments, "-c", key+"=false")
	}
	for _, feature := range parserDisabledFeatures() {
		arguments = append(arguments, "-c", "features."+feature+"=false")
	}
	return arguments
}

func parserDisabledFeatures() []string {
	return []string{
		"code_mode", "code_mode_only", "code_mode_tool_search", "code_mode_only_strict_3p_tools", "default_mode_request_user_input", "browser_use", "browser_use_external", "computer_use", "image_generation",
		"in_app_browser", "in_app_local_automation", "code_mode_host", "view_image",
		"sleep_tool", "goals", "skill_search", "skill_mcp_dependency_install",
		"tool_suggest", "auth_elicitation", "tool_call_mcp_elicitation",
		"realtime_conversation", "workspace_dependencies", "worktrees", "shell_snapshot",
	}
}
