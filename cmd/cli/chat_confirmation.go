package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
)

func executeChatCoreTurn(
	c *client.Client,
	input *chatInputReader,
	session string,
	baseMetadata map[string]any,
	lastJobID *int64,
	pendingInputs *[]string,
	line string,
	specialistRoleID string,
	interval time.Duration,
	progress bool,
	verbose bool,
	maxChars int,
	localShell bool,
	shellState *localShellState,
	ui *chatUI,
) bool {
	turnMetadata := cloneMetadata(baseMetadata)
	applySpecialistTurnOverrides(turnMetadata, specialistRoleID)
	turnMetadata["session_id"] = session
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		turnMetadata["client_cwd"] = cwd
		turnMetadata["host_env_cwd"] = cwd
	}
	applyHostTemporalMetadata(turnMetadata, time.Now())
	if *lastJobID > 0 {
		turnMetadata["parent_job_id"] = *lastJobID
	}

	job, err := c.Enqueue(context.Background(), line, model.PipelineChat, turnMetadata)
	if err != nil {
		if isDeterministicLocalActionReviewPrompt(line) && isLikelyCoreUnavailableError(err) {
			emitSystem(ui, "core service unavailable; skipped deterministic post-action review after local action")
			return false
		}
		fmt.Fprintf(os.Stderr, "error enqueueing turn: %v\n", err)
		return false
	}
	*lastJobID = job.ID
	emitSystem(ui, fmt.Sprintf("assistant thinking (job %d)...", job.ID))
	emitSystem(ui, queuedTurnHintText())

	details, quit, err := awaitInteractiveTurn(c, input, job.ID, interval, progress, verbose, maxChars, pendingInputs, ui)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error waiting for turn: %v\n", err)
		return false
	}
	if quit {
		return true
	}

	if strings.TrimSpace(details.Job.Result) != "" {
		if localShell {
			updateLocalShellStateFromAssistant(shellState, details.Job.Result)
		}
		emitAssistant(ui, strings.TrimSpace(details.Job.Result))
	}
	if strings.TrimSpace(details.Job.Error) != "" {
		emitAssistantError(ui, strings.TrimSpace(details.Job.Error))
	}
	return false
}

type confirmationDecision string

const (
	confirmationDecisionApprove confirmationDecision = "approve"
	confirmationDecisionReject  confirmationDecision = "reject"
	confirmationDecisionRevise  confirmationDecision = "revise"
)

var confirmationRejectPrefixPattern = regexp.MustCompile(`(?i)^\s*(?:no|n|nope|nah|negative|incorrect|not quite|not exactly|don't|do not|stop|cancel|never mind|nevermind)\b[\s,;:.\-]*(?:but\b[\s,;:.\-]*)?`)
var confirmationApprovePrefixPattern = regexp.MustCompile(`(?i)^\s*(?:yes|y|ok|okay|sure|approve|approved|go ahead|proceed|do it|run it|green light)\b[\s,;:.\-!]*`)
var confirmationRevisionLeadPattern = regexp.MustCompile(`(?i)^(?:but|however|instead|actually|rather|except)\b`)

func interpretConfirmationReply(input string) (confirmationDecision, string) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return confirmationDecisionReject, ""
	}

	lower := strings.ToLower(clean)
	if match := confirmationApprovePrefixPattern.FindString(clean); match != "" {
		remainder := strings.TrimSpace(clean[len(match):])
		if remainder != "" && confirmationRevisionLeadPattern.MatchString(strings.ToLower(remainder)) {
			return confirmationDecisionRevise, clean
		}
		return confirmationDecisionApprove, ""
	}

	negativeRemainder := strings.TrimSpace(confirmationRejectPrefixPattern.ReplaceAllString(clean, ""))
	if negativeRemainder != clean {
		return confirmationDecisionReject, negativeRemainder
	}
	for _, phrase := range []string{
		"no", "n", "nope", "nah", "negative", "incorrect", "not quite", "not exactly", "don't", "do not", "stop", "cancel", "never mind", "nevermind",
	} {
		if lower == phrase {
			return confirmationDecisionReject, ""
		}
	}

	return confirmationDecisionRevise, clean
}

func revisedChatActionCandidate(
	previous *chatActionCandidate,
	feedback string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
	shellState *localShellState,
) *chatActionCandidate {
	if previous == nil {
		return nil
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return previous
	}

	revised := buildChatActionCandidate(feedback, localMedia, localBrowser, localScreen, localShell, localAudio, shellState)
	if revised != nil && strings.TrimSpace(revised.Input) != "" {
		// Reinterpretation feedback for a core chat turn should not replace the original user request.
		if previous.Kind == "core_job" && revised.Kind == "core_job" {
			return previous
		}
		if revised.Kind != "core_job" || previous.Kind == "core_job" {
			return revised
		}
	}

	combinedInput := strings.TrimSpace(previous.Input + "\n" + feedback)
	if combinedInput == "" {
		return previous
	}
	combined := buildChatActionCandidate(combinedInput, localMedia, localBrowser, localScreen, localShell, localAudio, shellState)
	if combined != nil && strings.TrimSpace(combined.Input) != "" {
		return combined
	}

	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    previous.Kind,
		Input:   combinedInput,
		Summary: previous.Summary,
	})
}

func buildActionInterpretationPrompt(
	candidate *chatActionCandidate,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) string {
	capabilities := enabledAutomationCapabilities(localMedia, localBrowser, localScreen, localShell, localAudio)
	if candidate == nil {
		lines := []string{
			"Interpret the user's request before execution.",
			"Do not execute anything yet.",
			"Use recent conversation from this chat session to preserve context.",
		}
		if len(capabilities) > 0 {
			lines = append(lines, "", "Available local capabilities:")
			lines = append(lines, capabilities...)
		}
		lines = append(lines,
			"",
			"Safety constraints:",
			"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
			"- Do not remove/delete files.",
			"",
			"Respond in this structure:",
			"Interpretation: <your best understanding of user intent and goal>",
			"Questions: <early clarifying questions, or \"none\">",
			"Confirmation: <single concise confirmation request>",
		)
		return strings.Join(lines, "\n")
	}

	lines := []string{
		"Interpret the user's request before execution.",
		"Do not execute anything yet.",
		"Use recent conversation from this chat session to preserve context.",
		"",
		"Original request:",
		candidate.Input,
		"",
		"Preliminary routing guess:",
		candidate.Summary,
	}
	role := candidateSpecialistRole(candidate)
	lines = append(lines, "", "Assigned specialist:")
	lines = append(lines, specialist.DetailLines(role)...)
	if len(capabilities) > 0 {
		lines = append(lines, "", "Available local capabilities:")
		lines = append(lines, capabilities...)
	}
	lines = append(lines,
		"",
		"Safety constraints:",
		"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
		"- Do not remove/delete files.",
		"",
		"Respond in this structure:",
		"Interpretation: <your best understanding of user intent and goal>",
		"Questions: <early clarifying questions, or \"none\">",
		"Confirmation: <single concise confirmation request>",
	)
	return strings.Join(lines, "\n")
}

func buildActionReinterpretationPrompt(
	candidate *chatActionCandidate,
	feedback string,
	localMedia bool,
	localBrowser bool,
	localScreen bool,
	localShell bool,
	localAudio bool,
) string {
	capabilities := enabledAutomationCapabilities(localMedia, localBrowser, localScreen, localShell, localAudio)
	if candidate == nil {
		lines := []string{
			"Please reinterpret the user's request and ask any required clarifying questions before suggesting an action.",
			"Do not execute anything yet.",
			"Use recent conversation from this chat session to preserve context.",
		}
		if len(capabilities) > 0 {
			lines = append(lines, "", "Available local capabilities:")
			lines = append(lines, capabilities...)
		}
		lines = append(lines,
			"",
			"Safety constraints:",
			"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
			"- Do not remove/delete files.",
		)
		return strings.Join(lines, "\n")
	}
	cleanFeedback := strings.TrimSpace(feedback)
	lines := []string{
		"The user rejected an earlier interpretation for a local action request.",
		"Do not execute anything yet. Re-interpret intent and ask clarifying questions before action.",
		"Use recent conversation from this chat session to preserve context.",
		"",
		"Original request:",
		candidate.Input,
		"",
		"Rejected interpretation:",
		candidate.Summary,
	}
	role := candidateSpecialistRole(candidate)
	lines = append(lines, "", "Assigned specialist:")
	lines = append(lines, specialist.DetailLines(role)...)
	if len(capabilities) > 0 {
		lines = append(lines, "", "Available local capabilities:")
		lines = append(lines, capabilities...)
	}
	lines = append(lines,
		"",
		"Safety constraints:",
		"- Prefer non-sudo commands first. If elevated access is required, ask for sudo and explain why.",
		"- Do not remove/delete files.",
	)
	if cleanFeedback != "" {
		lines = append(lines, "", "User feedback on what was wrong:", cleanFeedback)
	} else {
		lines = append(lines, "", "User feedback on what was wrong:", "(none provided; user said no without details)")
		lines = append(lines, "Ask 2-4 targeted clarifying questions to bridge the gap before execution.")
	}
	lines = append(lines,
		"",
		"Respond in this structure:",
		"Interpretation: <your best revised interpretation>",
		"Questions: <early clarifying questions, or \"none\">",
		"Confirmation: <single prompt asking the user to confirm/correct before execution>",
	)
	return strings.Join(lines, "\n")
}
