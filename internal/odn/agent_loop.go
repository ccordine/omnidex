package odn

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	defaultAgentLoopSteps        = 12
	defaultAgentCommandsPerStep  = 4
	defaultAgentObservationChars = 2400
	defaultPlannerTimeout        = 60 * time.Second
	defaultCommandTimeout        = 90 * time.Second
)

var evidenceCommandsForObjective = func(string) []string { return nil }

type AgentCommandLoopConfig struct {
	MaxSteps            int
	MaxCommandsPerStep  int
	MaxObservationChars int
	PlannerTimeout      time.Duration
	CommandTimeout      time.Duration
	BeforeUserPrompt    func()
	AfterUserPrompt     func()
	InitialObservations []CommandObservation
	AllowClarification  bool
	RequireEvidence     bool
}

type AgentCommandLoopResult struct {
	Summary       string
	Events        []Event
	Transcript    []CommandObservation
	ExecutedCount int
	BlockedCount  int
	FailedCount   int
	Done          bool
}

type CommandObservation struct {
	Step    int
	Command string
	Status  string
	Stdout  string
	Stderr  string
	Error   string
}

func ExecuteAgentCommandLoop(ctx context.Context, session *Session, objective string, mode PermissionMode, in io.Reader, out io.Writer, client *OllamaClient, nextEventID func() string, runLogger *RunLogger) (AgentCommandLoopResult, error) {
	return ExecuteAgentCommandLoopWithConfig(ctx, session, objective, mode, in, out, client, nextEventID, runLogger, DefaultAgentCommandLoopConfig())
}

func DefaultAgentCommandLoopConfig() AgentCommandLoopConfig {
	return AgentCommandLoopConfig{
		MaxSteps:            defaultAgentLoopSteps,
		MaxCommandsPerStep:  defaultAgentCommandsPerStep,
		MaxObservationChars: defaultAgentObservationChars,
		PlannerTimeout:      defaultPlannerTimeout,
		CommandTimeout:      defaultCommandTimeout,
	}
}

func ExecuteAgentCommandLoopWithConfig(ctx context.Context, session *Session, objective string, mode PermissionMode, in io.Reader, out io.Writer, client *OllamaClient, nextEventID func() string, runLogger *RunLogger, cfg AgentCommandLoopConfig) (AgentCommandLoopResult, error) {
	cfg = normalizeAgentCommandLoopConfig(cfg)
	result := AgentCommandLoopResult{Events: make([]Event, 0, 32)}
	if client == nil {
		result.BlockedCount++
		result.Events = append(result.Events, Event{
			ID:        nextEventID(),
			Type:      "agent_blocked",
			Summary:   "No model client configured for command planning",
			Details:   map[string]string{"reason": "ollama_unavailable"},
			CreatedAt: nowUTC(),
		})
		result.Summary = "Execution blocked: no model is configured. Start without --no-ollama or set an Ollama endpoint/model."
		return result, nil
	}

	observations := make([]CommandObservation, 0, len(cfg.InitialObservations)+cfg.MaxSteps*cfg.MaxCommandsPerStep)
	observations = append(observations, cfg.InitialObservations...)
	if len(observations) > 0 {
		result.Transcript = append([]CommandObservation(nil), observations...)
	}
	if commands := evidenceCommandsForObjective(objective); len(commands) > 0 {
		for index, command := range commands {
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "deterministic_command_selected",
				Summary:   "Selected deterministic evidence command",
				Details:   map[string]string{"command": command, "index": fmt.Sprintf("%d", index+1)},
				CreatedAt: nowUTC(),
			})
			runCtx, cancel := context.WithTimeout(ctx, cfg.CommandTimeout)
			stdout, stderr, runErr := runShellCommand(runCtx, session.WorkspacePath, command)
			cancel()

			status := "success"
			errorText := ""
			if runErr != nil {
				status = "failed"
				errorText = runErr.Error()
				result.FailedCount++
			} else if !hasUsableCommandOutput(stdout, stderr) {
				status = "failed"
				errorText = "command produced no usable evidence"
				result.FailedCount++
			} else {
				result.ExecutedCount++
				result.Done = true
				result.Summary = truncateOutput(stdout)
			}
			result.Events = append(result.Events, Event{
				ID:      nextEventID(),
				Type:    "command_" + status,
				Summary: "Command " + status,
				Details: map[string]string{
					"step":    "0",
					"command": command,
					"stdout":  truncateOutput(stdout),
					"stderr":  truncateOutput(stderr),
					"error":   errorText,
				},
				CreatedAt: nowUTC(),
			})
			_ = runLogger.Log("agent_loop", "command_"+status, map[string]interface{}{
				"step":    0,
				"command": command,
				"stdout":  truncateOutput(stdout),
				"stderr":  truncateOutput(stderr),
				"error":   errorText,
			})
			observations = append(observations, CommandObservation{
				Step:    0,
				Command: command,
				Status:  status,
				Stdout:  truncateForObservation(stdout, cfg.MaxObservationChars),
				Stderr:  truncateForObservation(stderr, cfg.MaxObservationChars),
				Error:   errorText,
			})
			result.Transcript = append([]CommandObservation(nil), observations...)
			if result.Done {
				return result, nil
			}
		}
	}
	for step := 1; step <= cfg.MaxSteps; step++ {
		stepCtx, cancel := context.WithTimeout(ctx, cfg.PlannerTimeout)
		resp, err := client.ChatRaw(stepCtx, OllamaChatRequest{
			Messages: buildAgentPlannerMessages(session.WorkspacePath, objective, observations, cfg),
			Options: map[string]interface{}{
				"temperature": 0,
				"num_predict": 256,
			},
		})
		cancel()
		if err != nil {
			result.FailedCount++
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "planner_failed",
				Summary:   "Model command planner failed",
				Details:   map[string]string{"error": err.Error()},
				CreatedAt: nowUTC(),
			})
			result.Summary = fmt.Sprintf("Execution failed: model command planner failed: %v", err)
			return result, nil
		}

		_ = runLogger.Log("agent_loop", "planner_response", map[string]interface{}{
			"step":                  step,
			"request":               resp.RequestJSON,
			"response":              resp.ResponseJSON,
			"total_duration_ns":     resp.TotalDuration,
			"prompt_eval_count":     resp.PromptEvalCount,
			"completion_eval_count": resp.EvalCount,
		})

		commands, doneMessage, clarificationQuestion := parseAgentPlannerOutput(resp.Content, cfg.MaxCommandsPerStep)
		result.Events = append(result.Events, Event{
			ID:      nextEventID(),
			Type:    "planner_completed",
			Summary: "Model emitted next command batch",
			Details: map[string]string{
				"step":          fmt.Sprintf("%d", step),
				"command_count": fmt.Sprintf("%d", len(commands)),
				"done":          fmt.Sprintf("%t", doneMessage != ""),
				"ask":           fmt.Sprintf("%t", clarificationQuestion != ""),
			},
			CreatedAt: nowUTC(),
		})

		if clarificationQuestion != "" {
			if !cfg.AllowClarification {
				result.BlockedCount++
				result.Events = append(result.Events, Event{
					ID:        nextEventID(),
					Type:      "clarification_rejected",
					Summary:   "Clarification rejected by context plan",
					Details:   map[string]string{"step": fmt.Sprintf("%d", step), "question": clarificationQuestion},
					CreatedAt: nowUTC(),
				})
				observations = append(observations, CommandObservation{
					Step:   step,
					Status: "blocked",
					Error:  "ASK rejected: context plan did not allow clarification",
				})
				result.Transcript = append([]CommandObservation(nil), observations...)
				continue
			}
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "clarification_requested",
				Summary:   "Model requested user clarification",
				Details:   map[string]string{"step": fmt.Sprintf("%d", step), "question": clarificationQuestion},
				CreatedAt: nowUTC(),
			})
			runBeforeUserPrompt(cfg)
			answer, promptErr := PromptClarification(in, out, clarificationQuestion)
			runAfterUserPrompt(cfg)
			if promptErr != nil {
				return result, promptErr
			}
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "clarification_answered",
				Summary:   "User provided clarification",
				Details:   map[string]string{"step": fmt.Sprintf("%d", step), "answer": truncateOutput(answer)},
				CreatedAt: nowUTC(),
			})
			observations = append(observations, CommandObservation{
				Step:    step,
				Command: "ASK: " + clarificationQuestion,
				Status:  "user_input",
				Stdout:  truncateForObservation(answer, cfg.MaxObservationChars),
			})
			result.Transcript = append([]CommandObservation(nil), observations...)
			continue
		}

		if doneMessage != "" {
			if result.ExecutedCount == 0 && !hasObservedOutput(observations) {
				result.Events = append(result.Events, Event{
					ID:        nextEventID(),
					Type:      "planner_done_rejected",
					Summary:   "Model emitted DONE before any command succeeded",
					Details:   map[string]string{"step": fmt.Sprintf("%d", step), "done_message": truncateOutput(doneMessage)},
					CreatedAt: nowUTC(),
				})
				observations = append(observations, CommandObservation{
					Step:   step,
					Status: "blocked",
					Error:  "DONE rejected: no command has succeeded yet; run a command and verify stdout/stderr before DONE",
				})
				result.Transcript = append([]CommandObservation(nil), observations...)
				continue
			}
			if latestObservationBlocksDone(observations) {
				result.Events = append(result.Events, Event{
					ID:        nextEventID(),
					Type:      "planner_done_rejected",
					Summary:   "Model emitted DONE after unresolved command failure/block",
					Details:   map[string]string{"step": fmt.Sprintf("%d", step), "done_message": truncateOutput(doneMessage)},
					CreatedAt: nowUTC(),
				})
				observations = append(observations, CommandObservation{
					Step:   step,
					Status: "blocked",
					Error:  "DONE rejected: latest command was blocked/failed; recover with another command or report the missing evidence explicitly",
				})
				result.Transcript = append([]CommandObservation(nil), observations...)
				continue
			}
			if cfg.RequireEvidence && !hasObservedOutput(observations) {
				result.Events = append(result.Events, Event{
					ID:        nextEventID(),
					Type:      "planner_done_rejected",
					Summary:   "Model emitted DONE without captured command output",
					Details:   map[string]string{"step": fmt.Sprintf("%d", step), "done_message": truncateOutput(doneMessage)},
					CreatedAt: nowUTC(),
				})
				observations = append(observations, CommandObservation{
					Step:   step,
					Status: "blocked",
					Error:  "DONE rejected: fact question has no captured stdout/stderr; run a command that prints evidence",
				})
				result.Transcript = append([]CommandObservation(nil), observations...)
				continue
			}
			result.Done = true
			result.Summary = doneMessage
			return result, nil
		}
		if len(commands) == 0 {
			result.BlockedCount++
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "planner_blocked",
				Summary:   "Model produced no executable command lines",
				Details:   map[string]string{"step": fmt.Sprintf("%d", step), "raw_output": truncateOutput(resp.Content)},
				CreatedAt: nowUTC(),
			})
			observations = append(observations, CommandObservation{
				Step:   step,
				Status: "blocked",
				Error:  "planner output contained no executable command lines; emit only shell commands or DONE:",
			})
			result.Transcript = append([]CommandObservation(nil), observations...)
			continue
		}

		for _, commandLine := range commands {
			decision := EvaluateCommandPolicy(commandLine, session.WorkspacePath)
			if !decision.Allowed {
				result.BlockedCount++
				result.Events = append(result.Events, Event{
					ID:      nextEventID(),
					Type:    "policy_blocked",
					Summary: "Command blocked by policy",
					Details: map[string]string{
						"step":        fmt.Sprintf("%d", step),
						"command":     commandLine,
						"reason_code": decision.ReasonCode,
						"detail":      decision.Detail,
					},
					CreatedAt: nowUTC(),
				})
				observations = append(observations, CommandObservation{
					Step:    step,
					Command: commandLine,
					Status:  "blocked",
					Error:   decision.ReasonCode + ": " + decision.Detail,
				})
				result.Transcript = append([]CommandObservation(nil), observations...)
				break
			}

			if mode == PermissionAsk && commandLikelyWrites(commandLine) {
				runBeforeUserPrompt(cfg)
				approved, promptErr := PromptYesNo(in, out, fmt.Sprintf("Approve command [%s]? [y/N]: ", commandLine))
				runAfterUserPrompt(cfg)
				if promptErr != nil {
					return result, promptErr
				}
				if !approved {
					result.BlockedCount++
					result.Events = append(result.Events, Event{
						ID:        nextEventID(),
						Type:      "permission_denied",
						Summary:   "Command denied by user",
						Details:   map[string]string{"step": fmt.Sprintf("%d", step), "command": commandLine},
						CreatedAt: nowUTC(),
					})
					observations = append(observations, CommandObservation{
						Step:    step,
						Command: commandLine,
						Status:  "blocked",
						Error:   "user denied permission",
					})
					result.Transcript = append([]CommandObservation(nil), observations...)
					break
				}
			}

			runCtx, cancel := context.WithTimeout(ctx, cfg.CommandTimeout)
			stdout, stderr, runErr := runShellCommand(runCtx, session.WorkspacePath, commandLine)
			cancel()

			status := "success"
			errorText := ""
			if runErr != nil {
				status = "failed"
				errorText = runErr.Error()
				result.FailedCount++
			} else if cfg.RequireEvidence && !hasUsableCommandOutput(stdout, stderr) {
				status = "failed"
				errorText = "command produced no usable evidence"
				result.FailedCount++
			} else {
				result.ExecutedCount++
			}

			result.Events = append(result.Events, Event{
				ID:      nextEventID(),
				Type:    "command_" + status,
				Summary: "Command " + status,
				Details: map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"command": commandLine,
					"stdout":  truncateOutput(stdout),
					"stderr":  truncateOutput(stderr),
					"error":   errorText,
				},
				CreatedAt: nowUTC(),
			})
			_ = runLogger.Log("agent_loop", "command_"+status, map[string]interface{}{
				"step":    step,
				"command": commandLine,
				"stdout":  truncateOutput(stdout),
				"stderr":  truncateOutput(stderr),
				"error":   errorText,
			})

			observations = append(observations, CommandObservation{
				Step:    step,
				Command: commandLine,
				Status:  status,
				Stdout:  truncateForObservation(stdout, cfg.MaxObservationChars),
				Stderr:  truncateForObservation(stderr, cfg.MaxObservationChars),
				Error:   errorText,
			})
			result.Transcript = append([]CommandObservation(nil), observations...)
			if status == "failed" {
				break
			}
		}
	}

	result.Summary = fmt.Sprintf("Execution stopped after %d planner step(s): executed=%d blocked=%d failed=%d. The model did not emit DONE before the step budget ended.", cfg.MaxSteps, result.ExecutedCount, result.BlockedCount, result.FailedCount)
	return result, nil
}

func runBeforeUserPrompt(cfg AgentCommandLoopConfig) {
	if cfg.BeforeUserPrompt != nil {
		cfg.BeforeUserPrompt()
	}
}

func runAfterUserPrompt(cfg AgentCommandLoopConfig) {
	if cfg.AfterUserPrompt != nil {
		cfg.AfterUserPrompt()
	}
}

func hasObservedOutput(observations []CommandObservation) bool {
	for _, obs := range observations {
		if obs.Status != "success" && obs.Status != "user_input" {
			continue
		}
		if hasUsableCommandOutput(obs.Stdout, obs.Stderr) {
			return true
		}
	}
	return false
}

func hasUsableCommandOutput(stdout, stderr string) bool {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return false
	}
	lower := strings.ToLower(stdout)
	if lower == "null" || lower == "[]" || lower == "{}" || lower == "nan" {
		return false
	}
	return true
}

func latestObservationBlocksDone(observations []CommandObservation) bool {
	for i := len(observations) - 1; i >= 0; i-- {
		switch observations[i].Status {
		case "failed", "blocked":
			return true
		case "success":
			return false
		}
	}
	return false
}

func normalizeAgentCommandLoopConfig(cfg AgentCommandLoopConfig) AgentCommandLoopConfig {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = defaultAgentLoopSteps
	}
	if cfg.MaxCommandsPerStep <= 0 {
		cfg.MaxCommandsPerStep = defaultAgentCommandsPerStep
	}
	if cfg.MaxObservationChars <= 0 {
		cfg.MaxObservationChars = defaultAgentObservationChars
	}
	if cfg.PlannerTimeout <= 0 {
		cfg.PlannerTimeout = defaultPlannerTimeout
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = defaultCommandTimeout
	}
	return cfg
}

func buildAgentPlannerMessages(workspacePath, objective string, observations []CommandObservation, cfg AgentCommandLoopConfig) []OllamaMessage {
	system := strings.Join(withMinimalOutputContract(
		"Role: command planner.",
		"Goal: emit terminal commands.",
		"Default: shell for facts/files/tests/edits/web.",
		"Commands only. One per line. No markdown/prose/comments.",
		"Complete: one line only: DONE: <short result>.",
		"DONE only from observed stdout/stderr.",
		"ASK: only for user preference ambiguity.",
		"No ASK for public facts/exact commands.",
		"If blocked/failed: recover before DONE.",
		"Commands run exactly as emitted.",
		"No cd. Cwd resets each command. Use full paths or command flags.",
		"Exact command requested: emit exact command.",
		"Prefer small observable commands.",
		"Time zones: env TZ=Area/City date. Never date -d Area/City.",
		"Weather: wttr.in no-key JSON; country uses capital.",
		"Web: use curl --max-time 20.",
		fmt.Sprintf("Emit at most %d command lines per step.", cfg.MaxCommandsPerStep),
	), "\n")

	user := strings.Builder{}
	user.WriteString("Workspace: ")
	user.WriteString(workspacePath)
	user.WriteString("\nObjective:\n")
	user.WriteString(strings.TrimSpace(objective))
	user.WriteString("\n\nRecent command observations:\n")
	if len(observations) == 0 {
		user.WriteString("(none yet)\n")
	} else {
		start := len(observations) - 10
		if start < 0 {
			start = 0
		}
		for _, obs := range observations[start:] {
			user.WriteString(fmt.Sprintf("step=%d status=%s command=%q\n", obs.Step, obs.Status, obs.Command))
			if strings.TrimSpace(obs.Stdout) != "" {
				user.WriteString("stdout:\n")
				user.WriteString(obs.Stdout)
				user.WriteString("\n")
			}
			if strings.TrimSpace(obs.Stderr) != "" {
				user.WriteString("stderr:\n")
				user.WriteString(obs.Stderr)
				user.WriteString("\n")
			}
			if strings.TrimSpace(obs.Error) != "" {
				user.WriteString("error:\n")
				user.WriteString(obs.Error)
				user.WriteString("\n")
			}
		}
	}
	user.WriteString("\nNext output must be shell command lines only, ASK: if blocked by ambiguity, or DONE: if complete.")

	return []OllamaMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user.String()},
	}
}

func parseAgentPlannerOutput(raw string, maxCommands int) ([]string, string, string) {
	if maxCommands <= 0 {
		maxCommands = defaultAgentCommandsPerStep
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	commands := make([]string, 0, maxCommands)
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		clean = strings.Trim(clean, "`")
		if clean == "" || clean == "```" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(clean), "done:") {
			return nil, strings.TrimSpace(clean[len("done:"):]), ""
		}
		if strings.HasPrefix(strings.ToLower(clean), "ask:") {
			return nil, "", strings.TrimSpace(clean[len("ask:"):])
		}
		if strings.HasPrefix(clean, "#") {
			continue
		}
		commands = append(commands, clean)
		if len(commands) >= maxCommands {
			break
		}
	}
	return commands, "", ""
}

func truncateForObservation(raw string, maxChars int) string {
	trimmed := strings.TrimSpace(raw)
	if maxChars <= 0 {
		maxChars = defaultAgentObservationChars
	}
	if len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "..."
}
