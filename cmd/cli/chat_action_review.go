package main

import (
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/specialist"
)

func runDeterministicLocalActionReview(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	candidate *chatActionCandidate,
	actionOutput string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	reviewMetadata := deterministicLocalActionReviewMetadata(baseMetadata)
	prompt := buildDeterministicLocalActionReviewPrompt(candidate, actionOutput)
	if ui != nil {
		emitSystem(ui, formatLocalReviewHandoffTrace(candidate, actionOutput))
	}
	return executeChatCoreTurn(
		c,
		input,
		session,
		reviewMetadata,
		lastJobID,
		pendingInputs,
		prompt,
		specialist.RoleReviewVerificationSpecialist,
		interval,
		progress,
		verbose,
		maxChars,
		localShell,
		shellState,
		ui,
	)
}

func deterministicLocalActionReviewMetadata(baseMetadata map[string]any) map[string]any {
	reviewMetadata := cloneMetadata(baseMetadata)
	reviewMetadata["verification_mode"] = "force"
	if value, ok := reviewMetadata["verification_iterations"].(int); !ok || value < 2 {
		reviewMetadata["verification_iterations"] = 2
	}
	reviewMetadata["review_always"] = true
	return reviewMetadata
}

func buildDeterministicLocalActionReviewPrompt(candidate *chatActionCandidate, actionOutput string) string {
	request := "(missing original request)"
	kind := "unknown"
	if candidate != nil {
		if strings.TrimSpace(candidate.Input) != "" {
			request = strings.TrimSpace(candidate.Input)
		}
		if strings.TrimSpace(candidate.Kind) != "" {
			kind = strings.TrimSpace(candidate.Kind)
		}
	}
	output := strings.TrimSpace(actionOutput)
	if output == "" {
		output = "(no local action output captured)"
	}
	lines := []string{
		"Deterministic post-action review step (required):",
		"- You are in the review phase after a local action execution.",
		"- Do not skip this review.",
		"- Compare the original user request against the concrete execution output.",
		"- If the task is incomplete, explicitly start with `INCOMPLETE:` and state the exact next action required to continue from current state.",
		"- If the task is complete, explicitly start with `COMPLETE:` and provide the final answer.",
		"- If a next local shell command is required, include exactly one safe command in backticks.",
		"",
		"Original user request:",
		request,
		"",
		"Local capability kind:",
		kind,
		"",
		"Executed local action output:",
		output,
	}
	return strings.Join(lines, "\n")
}

func isDeterministicLocalActionReviewPrompt(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "Deterministic post-action review step")
}

func isLikelyCoreUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "client.timeout") ||
		strings.Contains(text, "connection reset")
}
