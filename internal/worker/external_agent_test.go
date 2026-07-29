package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

func TestSelectExternalAgentAppliesCursorConfig(t *testing.T) {
	t.Setenv("CURSOR_API_KEY", "cursor-key")
	cfg, err := agentconfig.FromStringMap(map[string]string{
		"agent_system": "cursor",
		"cursor_model": "composer-project",
	})
	if err != nil {
		t.Fatalf("parse agent config: %v", err)
	}
	agent, name, unavailable := selectExternalAgent(cfg)
	if unavailable != "" {
		t.Fatalf("expected cursor available, got %q", unavailable)
	}
	if name != "cursor_sdk" {
		t.Fatalf("expected cursor_sdk, got %q", name)
	}
	cursor, ok := agent.(*omni.CursorSDKAgent)
	if !ok {
		t.Fatalf("expected cursor sdk agent, got %T", agent)
	}
	if cursor.Model != "composer-project" {
		t.Fatalf("expected configured cursor model, got %q", cursor.Model)
	}
}

func TestSelectExternalAgentAppliesCodexConfig(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "codex-key")
	cfg, err := agentconfig.FromStringMap(map[string]string{
		"agent_system":           "codex",
		"codex_model":            "gpt-codex-project",
		"codex_reasoning_effort": "high",
		"codex_sandbox_mode":     "workspace-write",
		"codex_approval_policy":  "never",
		"codex_network_access":   "false",
		"codex_web_search_mode":  "disabled",
	})
	if err != nil {
		t.Fatalf("parse agent config: %v", err)
	}
	agent, name, unavailable := selectExternalAgent(cfg)
	if unavailable != "" {
		t.Fatalf("expected codex available, got %q", unavailable)
	}
	if name != "codex_sdk" {
		t.Fatalf("expected codex_sdk, got %q", name)
	}
	codex, ok := agent.(*omni.CodexSDKAgent)
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

func TestExternalAgentJobModeUsesGenericCLIForChat(t *testing.T) {
	job := model.Job{
		Pipeline: model.PipelineChat,
		Metadata: json.RawMessage(`{"client_cwd":"/tmp/project","agent_config":{"agent_system":"codex"}}`),
	}
	if got := externalAgentJobMode(job); got != "cli_agent_task" {
		t.Fatalf("externalAgentJobMode()=%q", got)
	}
	prompt := buildExternalAgentContext(job, map[string]string{"environment": "env summary"}, agentconfig.SystemCodex)
	if !strings.Contains(prompt, "bounded CLI agent task") {
		t.Fatalf("expected generic CLI prompt, got %q", prompt)
	}
	if strings.Contains(prompt, "scrum card") {
		t.Fatalf("generic prompt should not mention scrum cards: %q", prompt)
	}
}

func TestExternalAgentJobModeKeepsScrumPrompt(t *testing.T) {
	job := model.Job{
		Pipeline: "scrum",
		Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"cursor"}}`),
	}
	if got := externalAgentJobMode(job); got != "scrum_task" {
		t.Fatalf("externalAgentJobMode()=%q", got)
	}
	prompt := buildExternalAgentContext(job, nil, agentconfig.SystemCursor)
	if !strings.Contains(prompt, "bounded scrum card task") {
		t.Fatalf("expected scrum prompt, got %q", prompt)
	}
}
