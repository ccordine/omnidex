package omni

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/secrets"
)

func TestCursorSDKEnabledWithDBKeyOnly(t *testing.T) {
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "")
	t.Setenv("CURSOR_API_KEY", "")
	t.Setenv("OMNI_DISABLE_CURSOR_ARCHITECT", "")

	secrets.SetGlobal(secrets.NewResolver(secretsStoreStub{
		values: map[string]string{"cursor_api_key": "test-key"},
	}))
	t.Cleanup(func() { secrets.SetGlobal(nil) })

	if !CursorSDKEnabled(false) {
		t.Fatal("expected cursor enabled when DB key is configured")
	}
	if NewCursorSDKArchitectAgent(true) == nil {
		t.Fatal("expected cursor agent when DB key is configured")
	}
}

func TestCursorSDKDisabledWithoutKeyOrFlag(t *testing.T) {
	t.Setenv("OMNI_ENABLE_CURSOR_ARCHITECT", "")
	t.Setenv("CURSOR_API_KEY", "")
	secrets.SetGlobal(nil)

	if CursorSDKEnabled(false) {
		t.Fatal("expected cursor disabled without key or enable flag")
	}
}

func TestCursorSDKAgentApplyConfigUsesCursorModel(t *testing.T) {
	agent := &CursorSDKArchitectAgent{Model: "composer-default"}
	cfg, err := agentconfig.FromStringMap(map[string]string{
		"agent_system": "cursor",
		"cursor_model": "composer-project",
	})
	if err != nil {
		t.Fatalf("parse agent config: %v", err)
	}
	agent.ApplyConfig(cfg)
	if agent.Model != "composer-project" {
		t.Fatalf("expected configured cursor model, got %q", agent.Model)
	}
}

func TestCursorSDKLocalSessionFailsLoudlyOnPreflight(t *testing.T) {
	t.Setenv("HOST_AGENT_URL", "")
	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "true")
	missing := filepath.Join(t.TempDir(), "missing-node")
	t.Setenv("OMNI_CURSOR_NODE_BIN", missing)
	t.Setenv("OMNI_CURSOR_NPM_BIN", missing)

	agent := &CursorSDKArchitectAgent{
		APIKey:    "cursor-key",
		Model:     "composer-test",
		RunnerDir: t.TempDir(),
	}
	_, err := agent.NewExternalAgentSession(CursorArchitectAgentInput{})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	if !strings.Contains(err.Error(), "cursor host preflight failed") {
		t.Fatalf("expected loud cursor preflight error, got %v", err)
	}
}

func TestCodexSDKAgentApplyConfigUsesProjectValues(t *testing.T) {
	agent := &CodexSDKArchitectAgent{Model: "gpt-default"}
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
	agent.ApplyConfig(cfg)
	if agent.Model != "gpt-codex-project" {
		t.Fatalf("expected configured codex model, got %q", agent.Model)
	}
	if agent.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning effort, got %q", agent.ReasoningEffort)
	}
	if agent.SandboxMode != "workspace-write" {
		t.Fatalf("expected sandbox mode, got %q", agent.SandboxMode)
	}
	if agent.ApprovalPolicy != "never" {
		t.Fatalf("expected approval policy, got %q", agent.ApprovalPolicy)
	}
	if agent.NetworkAccess != "false" {
		t.Fatalf("expected network access, got %q", agent.NetworkAccess)
	}
	if agent.WebSearchMode != "disabled" {
		t.Fatalf("expected web search mode, got %q", agent.WebSearchMode)
	}
}

func TestCodexSDKLocalSessionFailsLoudlyOnPreflight(t *testing.T) {
	t.Setenv("HOST_AGENT_URL", "")
	t.Setenv("OMNI_EXTERNAL_AGENT_FORCE_LOCAL", "true")
	missing := filepath.Join(t.TempDir(), "missing-codex")
	t.Setenv("OMNI_CODEX_NODE_BIN", missing)
	t.Setenv("OMNI_CODEX_NPM_BIN", missing)
	t.Setenv("OMNI_CODEX_BIN", missing)

	agent := &CodexSDKArchitectAgent{
		APIKey:    "codex-key",
		Model:     "gpt-codex-project",
		RunnerDir: t.TempDir(),
		NodeBin:   missing,
		NPMBin:    missing,
		CodexBin:  missing,
	}
	_, err := agent.NewExternalAgentSession(CursorArchitectAgentInput{})
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	if !strings.Contains(err.Error(), "codex host preflight failed") {
		t.Fatalf("expected loud codex preflight error, got %v", err)
	}
}

type secretsStoreStub struct {
	values map[string]string
}

func (s secretsStoreStub) GetAPISecrets(context.Context) (map[string]string, error) {
	return s.values, nil
}
