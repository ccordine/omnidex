package worker

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/omni"
)

func TestExternalAgentFailureIsFatalForConfiguredExternalAgents(t *testing.T) {
	cases := []struct {
		name     string
		cfg      agentconfig.Config
		metadata json.RawMessage
		want     bool
	}{
		{
			name:     "nested external agent config",
			cfg:      agentconfig.FromJobMetadata(json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`)),
			metadata: json.RawMessage(`{"agent_config":{"agent_system":"codex"}}`),
			want:     true,
		},
		{
			name:     "strict native config",
			cfg:      agentconfig.FromStringMap(map[string]string{"agent_strict": "true"}),
			metadata: json.RawMessage(`{"agent_config":{"agent_strict":"true"}}`),
			want:     true,
		},
		{
			name:     "scrum execution agent metadata",
			cfg:      agentconfig.Config{},
			metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor"}`),
			want:     true,
		},
		{
			name:     "unconfigured general chat",
			cfg:      agentconfig.Config{},
			metadata: json.RawMessage(`{"source":"omni-web-chat"}`),
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := externalAgentFailureIsFatal(tc.cfg, tc.metadata); got != tc.want {
				t.Fatalf("externalAgentFailureIsFatal()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestSelectExternalAgentAppliesCursorConfig(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "cursor-key")
	cfg := agentconfig.FromStringMap(map[string]string{
		"agent_system": "cursor",
		"cursor_model": "composer-project",
	})
	agent, name, unavailable := selectExternalAgent(cfg, json.RawMessage(`{"agent_config":{"agent_system":"cursor","cursor_model":"composer-project"}}`))
	if unavailable != "" {
		t.Fatalf("expected cursor available, got %q", unavailable)
	}
	if name != "cursor_sdk" {
		t.Fatalf("expected cursor_sdk, got %q", name)
	}
	cursor, ok := agent.(*omni.CursorSDKArchitectAgent)
	if !ok {
		t.Fatalf("expected cursor sdk agent, got %T", agent)
	}
	if cursor.Model != "composer-project" {
		t.Fatalf("expected configured cursor model, got %q", cursor.Model)
	}
}

func TestSelectExternalAgentAppliesCodexConfig(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "codex-key")
	cfg := agentconfig.FromStringMap(map[string]string{
		"agent_system":           "codex",
		"codex_model":            "gpt-codex-project",
		"codex_reasoning_effort": "high",
		"codex_sandbox_mode":     "workspace-write",
		"codex_approval_policy":  "never",
		"codex_network_access":   "false",
		"codex_web_search_mode":  "disabled",
	})
	agent, name, unavailable := selectExternalAgent(cfg, json.RawMessage(`{"agent_config":{"agent_system":"codex","codex_model":"gpt-codex-project"}}`))
	if unavailable != "" {
		t.Fatalf("expected codex available, got %q", unavailable)
	}
	if name != "codex_sdk" {
		t.Fatalf("expected codex_sdk, got %q", name)
	}
	codex, ok := agent.(*omni.CodexSDKArchitectAgent)
	if !ok {
		t.Fatalf("expected codex sdk agent, got %T", agent)
	}
	if codex.Model != "gpt-codex-project" {
		t.Fatalf("expected configured codex model, got %q", codex.Model)
	}
	if codex.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning effort high, got %q", codex.ReasoningEffort)
	}
	if codex.SandboxMode != "workspace-write" {
		t.Fatalf("expected sandbox workspace-write, got %q", codex.SandboxMode)
	}
	if codex.ApprovalPolicy != "never" {
		t.Fatalf("expected approval policy never, got %q", codex.ApprovalPolicy)
	}
	if codex.NetworkAccess != "false" {
		t.Fatalf("expected network access false, got %q", codex.NetworkAccess)
	}
	if codex.WebSearchMode != "disabled" {
		t.Fatalf("expected web search disabled, got %q", codex.WebSearchMode)
	}
}
