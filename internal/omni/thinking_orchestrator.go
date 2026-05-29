package omni

import (
	"context"
	"fmt"
	"strings"
)

type ThinkingAction string

const (
	ThinkingActionDirectAnswer     ThinkingAction = "direct_answer"
	ThinkingActionInvokeExecution  ThinkingAction = "invoke_execution"
	ThinkingActionContinueThinking ThinkingAction = "continue"
)

// ThinkingTurnOutcome is the pilot's decision at turn entry.
type ThinkingTurnOutcome struct {
	Action            ThinkingAction
	DirectAnswer      string
	ExecutionPrompt   string
	ExecutionToolTask string
	ChannelID         string
	Conclusion        string
	Messages          []ThoughtMessage
}

// ThinkingToolDeps supplies live tools the pilot may invoke while thinking.
type ThinkingToolDeps struct {
	WebSearch    WebSearchService
	MemorySearch func(ctx context.Context, query string) (string, error)
}

type thinkingChannelMode string

const (
	thinkingModeEntry    thinkingChannelMode = "entry"
	thinkingModeRecovery thinkingChannelMode = "recovery"
)

func thinkingEntryMaxStepsFromEnv() int {
	steps := envIntOrDefault("OMNI_THINKING_ENTRY_MAX_STEPS", 12)
	if steps <= 0 {
		return 12
	}
	return steps
}

func (s OllamaThinkingService) OrchestrateTurn(ctx context.Context, input ThinkingInput, onEvent func(StructuredCommandEvent)) (ThinkingTurnOutcome, error) {
	input.Trigger = firstNonEmpty(strings.TrimSpace(input.Trigger), "turn_entry")
	result, err := s.runChannel(ctx, input, thinkingModeEntry, thinkingEntryMaxStepsFromEnv(), onEvent)
	if err != nil {
		return ThinkingTurnOutcome{}, err
	}
	return thinkingTurnOutcomeFromResult(result, input), nil
}

func (s OllamaThinkingService) Reason(ctx context.Context, input ThinkingInput, onEvent func(StructuredCommandEvent)) (ThinkingResult, error) {
	if s.Client == nil {
		return ThinkingResult{}, fmt.Errorf("thinking service client is required")
	}
	return s.runChannel(ctx, input, thinkingModeRecovery, s.maxSteps(), onEvent)
}

func (s OllamaThinkingService) maxSteps() int {
	if s.MaxSteps <= 0 {
		return defaultThinkingMaxSteps
	}
	return s.MaxSteps
}

func thinkingTurnOutcomeFromResult(result ThinkingResult, input ThinkingInput) ThinkingTurnOutcome {
	outcome := ThinkingTurnOutcome{
		Action:            result.Action,
		DirectAnswer:      strings.TrimSpace(result.DirectAnswer),
		ExecutionPrompt:   strings.TrimSpace(result.ExecutionPrompt),
		ExecutionToolTask: firstNonEmpty(strings.TrimSpace(result.ExecutionToolTask), strings.TrimSpace(result.RecoveryToolTask)),
		ChannelID:         result.ChannelID,
		Conclusion:        result.Conclusion,
		Messages:          result.Messages,
	}
	if outcome.Action == "" {
		switch {
		case outcome.DirectAnswer != "":
			outcome.Action = ThinkingActionDirectAnswer
		case outcome.ExecutionPrompt != "" || outcome.ExecutionToolTask != "":
			outcome.Action = ThinkingActionInvokeExecution
		case input.Trigger == "turn_entry" && strings.TrimSpace(result.Conclusion) != "" && !promptRequestsImplementationArchitecture(strings.ToLower(input.UserPrompt)):
			outcome.Action = ThinkingActionDirectAnswer
			outcome.DirectAnswer = result.Conclusion
		default:
			outcome.Action = ThinkingActionInvokeExecution
			if outcome.ExecutionPrompt == "" {
				outcome.ExecutionPrompt = input.UserPrompt
			}
		}
	}
	if outcome.Action == ThinkingActionDirectAnswer && outcome.DirectAnswer == "" {
		outcome.DirectAnswer = firstNonEmpty(result.Conclusion, "I could not produce a direct answer.")
	}
	if outcome.Action == ThinkingActionInvokeExecution && outcome.ExecutionPrompt == "" {
		outcome.ExecutionPrompt = input.UserPrompt
	}
	return outcome
}

func thinkingSystemPrompt(mode thinkingChannelMode) string {
	schema := `{"thought":"internal reasoning","tool":"","tool_input":"","done":false,"action":"","direct_answer":"","execution_prompt":"","execution_tool_task":"","conclusion":"","recovery_tool_task":"","research_query":""}`
	tools := []string{
		"  active_prompt — authoritative user request",
		"  memories — loaded session memory snippets",
		"  memory_search — query postgres/session memory (tool_input=search query)",
		"  prep_context — documentation/memory briefs for this turn",
		"  web_search — run live web search (tool_input=query)",
		"  observations — recent command observations",
		"  project_map — planned file tree",
		"  loop_state — loop monitor status",
		"  objectives — objective ledger",
	}
	switch mode {
	case thinkingModeEntry:
		return strings.Join([]string{
			"You are the Omni pilot: the manager-of-managers and sole entry point for every user turn.",
			"You think in this isolated channel before anything reaches the user or execution subsystems.",
			"Your job is to decide what happens next: answer directly, research, recall memory, or delegate to execution.",
			"Return JSON only with schema:",
			schema,
			"Available tools (set tool + tool_input, then continue on next step after tool_result):",
			strings.Join(tools, "\n"),
			"When done=true, set action to one of:",
			"  direct_answer — you can answer the user now; set direct_answer with the final user-facing response.",
			"  invoke_execution — build/code/shell/project work is required; set execution_prompt (refined objective) and optional execution_tool_task.",
			"Use direct_answer for questions you can answer from tools, memory, or web_search evidence.",
			"Use invoke_execution for app builds, file edits, installs, tests, and multi-step project work.",
			"You may call web_search or memory_search multiple times before deciding.",
			"Do not tell the user to run commands manually when invoke_execution is appropriate.",
		}, "\n")
	default:
		return strings.Join([]string{
			"You are the internal recovery pilot for Omni.",
			"You diagnose stuck execution and plan recovery in this isolated thought channel.",
			"Return JSON only with schema:",
			schema,
			"Available tools:",
			strings.Join(tools, "\n"),
			"When done=true, set action=invoke_execution with recovery_tool_task or execution_tool_task describing the next concrete execution step.",
			"Set conclusion with your diagnosis.",
		}, "\n")
	}
}

func thinkingContextInstructions(mode thinkingChannelMode) []string {
	switch mode {
	case thinkingModeEntry:
		return []string{
			"You are the turn entry pilot; decide direct_answer vs invoke_execution.",
			"Use web_search or memory_search when the user asks for current facts, docs, or prior project context.",
			"Use direct_answer when evidence is sufficient; otherwise invoke_execution with a refined execution_prompt.",
		}
	default:
		return []string{
			"Diagnose why execution is stuck relative to active_prompt.",
			"Use tools to inspect evidence before concluding.",
			"Prefer execution_tool_task or recovery_tool_task naming concrete next file/command work.",
		}
	}
}

func outcomeFieldsFromPayload(mode thinkingChannelMode, payload thinkingStepPayload, lastRecovery, lastResearch string) (ThinkingAction, string, string, string) {
	action := ThinkingAction(strings.TrimSpace(payload.Action))
	directAnswer := strings.TrimSpace(payload.DirectAnswer)
	executionPrompt := strings.TrimSpace(payload.ExecutionPrompt)
	executionToolTask := firstNonEmpty(
		strings.TrimSpace(payload.ExecutionToolTask),
		strings.TrimSpace(payload.RecoveryToolTask),
		lastRecovery,
	)
	if mode == thinkingModeRecovery {
		return ThinkingActionInvokeExecution, directAnswer, executionPrompt, executionToolTask
	}
	return action, directAnswer, executionPrompt, executionToolTask
}

func thinkingResponseFormatProperties() map[string]interface{} {
	return map[string]interface{}{
		"thought":              map[string]interface{}{"type": "string"},
		"tool":                 map[string]interface{}{"type": "string"},
		"tool_input":           map[string]interface{}{"type": "string"},
		"done":                 map[string]interface{}{"type": "boolean"},
		"action":               map[string]interface{}{"type": "string"},
		"direct_answer":        map[string]interface{}{"type": "string"},
		"execution_prompt":     map[string]interface{}{"type": "string"},
		"execution_tool_task":  map[string]interface{}{"type": "string"},
		"conclusion":           map[string]interface{}{"type": "string"},
		"recovery_tool_task":   map[string]interface{}{"type": "string"},
		"research_query":       map[string]interface{}{"type": "string"},
	}
}
