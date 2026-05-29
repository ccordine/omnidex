package omni

import "strings"

// ActivePromptContext is the programmatic source of truth for what the user asked for
// in the current turn. Every police-tier validator and specialist should receive it.
type ActivePromptContext struct {
	UserPrompt         string   `json:"user_prompt"`
	ToolTask           string   `json:"tool_task,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	ObjectiveSummary   string   `json:"objective_summary,omitempty"`
}

func NewActivePromptContext(prompt, toolTask string, criteria []string) ActivePromptContext {
	return ActivePromptContext{
		UserPrompt:         strings.TrimSpace(prompt),
		ToolTask:           strings.TrimSpace(toolTask),
		AcceptanceCriteria: append([]string(nil), criteria...),
	}
}

func (a ActivePromptContext) CombinedText() string {
	parts := []string{strings.TrimSpace(a.UserPrompt)}
	if task := strings.TrimSpace(a.ToolTask); task != "" {
		parts = append(parts, task)
	}
	if summary := strings.TrimSpace(a.ObjectiveSummary); summary != "" {
		parts = append(parts, summary)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func activePromptFromArchitectContract(contract ImplementationArchitectContract) ActivePromptContext {
	return NewActivePromptContext(
		architectContractPrompt(contract),
		contract.SourceToolTask,
		contract.AcceptanceCriteria,
	)
}

func activePromptPolicyLines() []string {
	return []string{
		"active_prompt.user_prompt is the authoritative current user request for this turn.",
		"Reject or revise any generated source, test, package metadata, or final answer that implements a different app domain than active_prompt.",
		"Music-studio, notes-app, graphing-calculator, and other prior project patterns are foreign unless active_prompt explicitly requests them.",
		"Do not treat session memories, playbooks, or reference history as permission to reuse a prior app's UI or domain.",
	}
}
