package omni

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type CommandRunResult struct {
	Summary         string
	Events          []Event
	ExecutedCount   int
	BlockedCount    int
	FailedCount     int
	GeneratedOutput string
}

func ExecuteLinuxCommandTool(ctx context.Context, client *OllamaClient, userInput string, mode PermissionMode, in io.Reader, out io.Writer, workspacePath string, nextEventID func() string, runLogger *RunLogger) (CommandRunResult, error) {
	source := "ollama"
	generatedOutput := ""
	commands := []string{}

	if client == nil {
		source = "deterministic"
		commands = extractDeterministicCommandLines(userInput)
		generatedOutput = strings.Join(commands, "\n")
		_ = runLogger.Log("linux_command", "deterministic_command_parse", map[string]interface{}{
			"command_count": len(commands),
			"user_input":    userInput,
		})
	} else {
		systemPrompt := MinimalOutputContract + " Output shell commands only. One per line. No markdown/explanation/comments. Max 3."
		userPrompt := "Workspace: " + workspacePath + "\nUser request:\n" + strings.TrimSpace(userInput)

		resp, err := client.ChatRaw(ctx, OllamaChatRequest{
			Messages: []OllamaMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
			Options: map[string]interface{}{
				"temperature": 0,
			},
		})
		if err != nil {
			fallbackCommands := extractDeterministicCommandLines(userInput)
			if len(fallbackCommands) == 0 {
				return CommandRunResult{}, err
			}
			source = "deterministic_after_ollama_error"
			commands = fallbackCommands
			generatedOutput = strings.Join(commands, "\n")
			_ = runLogger.Log("linux_command", "ollama_error_deterministic_fallback", map[string]interface{}{
				"error":         err.Error(),
				"command_count": len(commands),
			})
		} else {
			_ = runLogger.Log("linux_command", "llm_call", map[string]interface{}{
				"request":               resp.RequestJSON,
				"response":              resp.ResponseJSON,
				"total_duration_ns":     resp.TotalDuration,
				"prompt_eval_count":     resp.PromptEvalCount,
				"completion_eval_count": resp.EvalCount,
			})
			generatedOutput = resp.Content
			commands = parseCommandLines(resp.Content)
		}
	}
	events := []Event{{
		ID:      nextEventID(),
		Type:    "tool_generated",
		Summary: "linux_command generated candidate command lines",
		Details: map[string]string{
			"tool_id":       "linux_command",
			"command_count": fmt.Sprintf("%d", len(commands)),
			"source":        source,
		},
		CreatedAt: nowUTC(),
	}}

	if len(commands) == 0 {
		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "tool_blocked",
			Summary:   "linux_command produced no executable commands",
			Details:   map[string]string{"tool_id": "linux_command"},
			CreatedAt: nowUTC(),
		})
		return CommandRunResult{
			Summary:         "linux_command produced no commands. Use an explicit shell request such as: run pwd",
			Events:          events,
			GeneratedOutput: generatedOutput,
		}, nil
	}

	if len(commands) > 3 {
		commands = commands[:3]
		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "tool_capped",
			Summary:   "Command list truncated to deterministic max of 3",
			Details:   map[string]string{"tool_id": "linux_command"},
			CreatedAt: nowUTC(),
		})
	}

	result := CommandRunResult{Events: events, GeneratedOutput: generatedOutput}
	for index, commandLine := range commands {
		decision := EvaluateCommandPolicy(commandLine, workspacePath)
		if !decision.Allowed {
			result.BlockedCount++
			result.Events = append(result.Events, Event{
				ID:      nextEventID(),
				Type:    "policy_blocked",
				Summary: "Command blocked by policy",
				Details: map[string]string{
					"tool_id":       "linux_command",
					"command_index": fmt.Sprintf("%d", index+1),
					"command":       commandLine,
					"reason_code":   decision.ReasonCode,
					"detail":        decision.Detail,
				},
				CreatedAt: nowUTC(),
			})
			continue
		}

		needsWriteApproval := mode == PermissionAsk && commandLikelyWrites(commandLine)
		if needsWriteApproval {
			approved, promptErr := PromptYesNo(in, out, fmt.Sprintf("Approve command [%s]? [y/N]: ", commandLine))
			if promptErr != nil {
				return result, promptErr
			}
			if !approved {
				result.BlockedCount++
				result.Events = append(result.Events, Event{
					ID:        nextEventID(),
					Type:      "permission_denied",
					Summary:   "Command denied by user",
					Details:   map[string]string{"tool_id": "linux_command", "command": commandLine},
					CreatedAt: nowUTC(),
				})
				continue
			}
			result.Events = append(result.Events, Event{
				ID:        nextEventID(),
				Type:      "permission_granted",
				Summary:   "Command approved by user",
				Details:   map[string]string{"tool_id": "linux_command", "command": commandLine},
				CreatedAt: nowUTC(),
			})
		}

		runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		stdout, stderr, runErr := runShellCommand(runCtx, workspacePath, commandLine)
		cancel()

		if runErr != nil {
			result.FailedCount++
			result.Events = append(result.Events, Event{
				ID:      nextEventID(),
				Type:    "command_failed",
				Summary: "Command execution failed",
				Details: map[string]string{
					"tool_id":       "linux_command",
					"command_index": fmt.Sprintf("%d", index+1),
					"command":       commandLine,
					"error":         runErr.Error(),
					"stdout":        truncateOutput(stdout),
					"stderr":        truncateOutput(stderr),
				},
				CreatedAt: nowUTC(),
			})
			_ = runLogger.Log("linux_command", "command_failed", map[string]interface{}{
				"command": commandLine,
				"error":   runErr.Error(),
				"stdout":  truncateOutput(stdout),
				"stderr":  truncateOutput(stderr),
			})
			continue
		}

		result.ExecutedCount++
		result.Events = append(result.Events, Event{
			ID:      nextEventID(),
			Type:    "command_executed",
			Summary: "Command executed",
			Details: map[string]string{
				"tool_id":       "linux_command",
				"command_index": fmt.Sprintf("%d", index+1),
				"command":       commandLine,
				"stdout":        truncateOutput(stdout),
				"stderr":        truncateOutput(stderr),
			},
			CreatedAt: nowUTC(),
		})
		_ = runLogger.Log("linux_command", "command_executed", map[string]interface{}{
			"command": commandLine,
			"stdout":  truncateOutput(stdout),
			"stderr":  truncateOutput(stderr),
		})
	}

	result.Summary = fmt.Sprintf("linux_command finished: executed=%d blocked=%d failed=%d", result.ExecutedCount, result.BlockedCount, result.FailedCount)
	return result, nil
}

func parseCommandLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		clean = strings.TrimPrefix(clean, "`")
		clean = strings.TrimSuffix(clean, "`")
		if strings.HasPrefix(clean, "#") {
			continue
		}
		if strings.HasPrefix(clean, "-") && strings.Contains(clean, " ") {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func extractDeterministicCommandLines(userInput string) []string {
	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return []string{}
	}

	lines := strings.Split(trimmed, "\n")
	commands := make([]string, 0, len(lines))
	for _, line := range lines {
		command, ok := extractDeterministicCommandLine(line)
		if !ok {
			continue
		}
		commands = append(commands, command)
		if len(commands) >= 3 {
			break
		}
	}
	return commands
}

func extractDeterministicCommandLine(line string) (string, bool) {
	clean := strings.TrimSpace(line)
	if clean == "" {
		return "", false
	}
	clean = strings.TrimPrefix(clean, "$ ")
	clean = strings.Trim(clean, "`")

	if isAllowlistedCommandRoot(clean) {
		return clean, true
	}
	return "", false
}

func isAllowlistedCommandRoot(commandLine string) bool {
	parts := strings.Fields(strings.TrimSpace(commandLine))
	if len(parts) == 0 {
		return false
	}
	_, ok := allowedCommandRoots[parts[0]]
	return ok
}

func runShellCommand(ctx context.Context, workspacePath, commandLine string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-o", "pipefail", "-lc", commandLine)
	cmd.Dir = workspacePath

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func truncateOutput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 1000 {
		return trimmed
	}
	return trimmed[:1000] + "..."
}

func commandLikelyWrites(commandLine string) bool {
	parts := strings.Fields(strings.TrimSpace(commandLine))
	if len(parts) == 0 {
		return false
	}
	readOnlyRoots := map[string]struct{}{
		"ls":     {},
		"pwd":    {},
		"cat":    {},
		"echo":   {},
		"printf": {},
		"find":   {},
		"rg":     {},
		"grep":   {},
		"head":   {},
		"tail":   {},
		"sed":    {},
		"awk":    {},
		"date":   {},
		"whoami": {},
		"uname":  {},
		"git":    {},
		"go":     {},
	}
	_, readOnly := readOnlyRoots[parts[0]]
	return !readOnly
}
