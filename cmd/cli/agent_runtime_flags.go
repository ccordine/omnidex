package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
)

type cliAgentRuntimeFlags struct {
	AgentSystem          string
	AgentModel           string
	AgentStrict          bool
	CursorModel          string
	CodexModel           string
	CodexReasoningEffort string
	CodexSandboxMode     string
	CodexApprovalPolicy  string
	CodexNetworkAccess   string
	CodexWebSearchMode   string
}

type cliAgentRuntimeFlagPointers struct {
	AgentSystem          *string
	AgentModel           *string
	AgentStrict          *bool
	CursorModel          *string
	CodexModel           *string
	CodexReasoningEffort *string
	CodexSandboxMode     *string
	CodexApprovalPolicy  *string
	CodexNetworkAccess   *string
	CodexWebSearchMode   *string
}

func registerCLIAgentRuntimeFlags(fs *flag.FlagSet) cliAgentRuntimeFlagPointers {
	return cliAgentRuntimeFlagPointers{
		AgentSystem:          fs.String("agent", "", "override execution agent for this run: omnidex|cursor|codex"),
		AgentModel:           fs.String("agent-model", "", "override model for the selected external agent (requires --agent cursor|codex)"),
		AgentStrict:          fs.Bool("agent-strict", false, "mark this run as strict external-agent execution"),
		CursorModel:          fs.String("cursor-model", "", "override Cursor SDK model for this run"),
		CodexModel:           fs.String("codex-model", "", "override Codex SDK model for this run"),
		CodexReasoningEffort: fs.String("codex-reasoning-effort", "", "Codex reasoning effort: minimal|low|medium|high|xhigh"),
		CodexSandboxMode:     fs.String("codex-sandbox", "", "Codex sandbox mode: read-only|workspace-write|danger-full-access"),
		CodexApprovalPolicy:  fs.String("codex-approval", "", "Codex approval policy: never|on-request|on-failure|untrusted"),
		CodexNetworkAccess:   fs.String("codex-network", "", "Codex network access: true|false"),
		CodexWebSearchMode:   fs.String("codex-web-search", "", "Codex web search mode: disabled|cached|live"),
	}
}

func (p cliAgentRuntimeFlagPointers) Values() cliAgentRuntimeFlags {
	value := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return strings.TrimSpace(*ptr)
	}
	boolValue := func(ptr *bool) bool {
		return ptr != nil && *ptr
	}
	return cliAgentRuntimeFlags{
		AgentSystem:          value(p.AgentSystem),
		AgentModel:           value(p.AgentModel),
		AgentStrict:          boolValue(p.AgentStrict),
		CursorModel:          value(p.CursorModel),
		CodexModel:           value(p.CodexModel),
		CodexReasoningEffort: value(p.CodexReasoningEffort),
		CodexSandboxMode:     value(p.CodexSandboxMode),
		CodexApprovalPolicy:  value(p.CodexApprovalPolicy),
		CodexNetworkAccess:   value(p.CodexNetworkAccess),
		CodexWebSearchMode:   value(p.CodexWebSearchMode),
	}
}

func cliAgentRuntimeConfigFromFlags(flags cliAgentRuntimeFlags) (*cliAgentRuntimeConfig, error) {
	cfg := newCLIAgentRuntimeConfig()
	if strings.TrimSpace(flags.AgentSystem) != "" {
		if err := cfg.Set("agent_system", flags.AgentSystem); err != nil {
			return nil, err
		}
	}
	if flags.AgentStrict {
		if err := cfg.Set("agent_strict", "true"); err != nil {
			return nil, err
		}
	}
	for key, value := range map[string]string{
		"cursor_model":           flags.CursorModel,
		"codex_model":            flags.CodexModel,
		"codex_reasoning_effort": flags.CodexReasoningEffort,
		"codex_sandbox_mode":     flags.CodexSandboxMode,
		"codex_approval_policy":  flags.CodexApprovalPolicy,
		"codex_network_access":   flags.CodexNetworkAccess,
		"codex_web_search_mode":  flags.CodexWebSearchMode,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := cfg.Set(key, value); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(flags.AgentModel) == "" {
		return cfg, nil
	}
	switch cfg.AgentSystemOverride() {
	case agentconfig.SystemCursor:
		if existing := cfg.values["cursor_model"]; existing != "" && existing != strings.TrimSpace(flags.AgentModel) {
			return nil, fmt.Errorf("--agent-model conflicts with --cursor-model")
		}
	case agentconfig.SystemCodex:
		if existing := cfg.values["codex_model"]; existing != "" && existing != strings.TrimSpace(flags.AgentModel) {
			return nil, fmt.Errorf("--agent-model conflicts with --codex-model")
		}
	}
	if err := cfg.SetActiveAgentModel(flags.AgentModel); err != nil {
		return nil, err
	}
	return cfg, nil
}
