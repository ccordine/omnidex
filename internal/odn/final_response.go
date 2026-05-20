package odn

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const defaultFinalResponderTimeout = 40 * time.Second

var finalReviewTokenPattern = regexp.MustCompile(`[a-z0-9]{3,}`)

type FinalAssistantResponseReviewInput struct {
	UserInput string
	Response  string
	Evidence  []string
}

type FinalAssistantResponseReview struct {
	Passed     bool
	Confidence int
	Feedback   string
	Response   string
}

func BuildFinalResponderMessages(workspacePath, userInput string, result AgentCommandLoopResult) []OllamaMessage {
	return []OllamaMessage{
		{
			Role: "system",
			Content: strings.Join(withMinimalOutputContract(
				"Role: final responder.",
				"Use only provided execution facts.",
				"If stdout/stderr are empty, say no captured output.",
				"Audit tone. No success gloss.",
				"Recap: tried; did not work; worked; final.",
				"Regular answer last.",
				"Do not mention workspace path unless asked.",
				"No advice unless task incomplete.",
				"Max 8 short lines.",
			), "\n"),
		},
		{
			Role: "user",
			Content: strings.Join([]string{
				"Workspace: " + workspacePath,
				"User request: " + strings.TrimSpace(userInput),
				"Execution summary: " + strings.TrimSpace(result.Summary),
				fmt.Sprintf("Done: %t Executed: %d Blocked: %d Failed: %d", result.Done, result.ExecutedCount, result.BlockedCount, result.FailedCount),
				"Transcript:",
				formatCommandTranscript(result.Transcript, 12),
			}, "\n"),
		},
	}
}

func FinalizeAgentResponse(ctx context.Context, client *OllamaClient, workspacePath, userInput string, result AgentCommandLoopResult, runLogger *RunLogger) (string, error) {
	if client == nil {
		return result.Summary, nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultFinalResponderTimeout)
	defer cancel()

	resp, err := client.ChatRaw(ctx, OllamaChatRequest{
		Messages: BuildFinalResponderMessages(workspacePath, userInput, result),
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 160,
		},
	})
	if err != nil {
		return result.Summary, err
	}
	_ = runLogger.Log("final_responder", "llm_call", map[string]interface{}{
		"request":               resp.RequestJSON,
		"response":              resp.ResponseJSON,
		"total_duration_ns":     resp.TotalDuration,
		"prompt_eval_count":     resp.PromptEvalCount,
		"completion_eval_count": resp.EvalCount,
	})
	return guardFinalResponse(strings.TrimSpace(resp.Content), result), nil
}

func ReviewFinalAssistantResponse(input FinalAssistantResponseReviewInput) FinalAssistantResponseReview {
	userInput := strings.TrimSpace(input.UserInput)
	response := strings.TrimSpace(input.Response)
	if response == "" {
		return FinalAssistantResponseReview{
			Passed:     false,
			Confidence: 0,
			Feedback:   "final response was empty",
			Response:   "I could not produce a response for the current request.",
		}
	}

	evidenceText := strings.TrimSpace(strings.Join(input.Evidence, "\n"))
	if structuredFinalAnswerGivesInstructionsInsteadOfCompletion(userInput, response) {
		return FinalAssistantResponseReview{
			Passed:     false,
			Confidence: 25,
			Feedback:   "final response gives user instructions for a request that should have been executed",
			Response:   buildFinalReviewCorrection(userInput, response, evidenceText),
		}
	}

	if finalResponseLooksOffTask(userInput, response, evidenceText) {
		return FinalAssistantResponseReview{
			Passed:     false,
			Confidence: 35,
			Feedback:   "final response appears weakly related to the current user request",
			Response:   buildFinalReviewCorrection(userInput, response, evidenceText),
		}
	}

	if structuredTextSuggestsFalseCapabilityLimit(response) && finalRequestLikelyNeedsTools(userInput) {
		return FinalAssistantResponseReview{
			Passed:     false,
			Confidence: 40,
			Feedback:   "final response claims a capability limit where local tools or public sources may be available",
			Response:   buildFinalReviewCorrection(userInput, response, evidenceText),
		}
	}

	return FinalAssistantResponseReview{
		Passed:     true,
		Confidence: 100,
		Feedback:   "deterministic final response review passed",
		Response:   response,
	}
}

func formatCommandTranscript(transcript []CommandObservation, maxItems int) string {
	if len(transcript) == 0 {
		return "(none)"
	}
	if maxItems <= 0 {
		maxItems = len(transcript)
	}
	start := len(transcript) - maxItems
	if start < 0 {
		start = 0
	}

	lines := make([]string, 0, len(transcript)-start)
	for _, obs := range transcript[start:] {
		parts := []string{
			fmt.Sprintf("step=%d", obs.Step),
			"status=" + obs.Status,
		}
		if strings.TrimSpace(obs.Command) != "" {
			parts = append(parts, "command="+obs.Command)
		}
		if strings.TrimSpace(obs.Stdout) != "" {
			parts = append(parts, "stdout="+truncateOutput(obs.Stdout))
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			parts = append(parts, "stderr="+truncateOutput(obs.Stderr))
		}
		if strings.TrimSpace(obs.Error) != "" {
			parts = append(parts, "error="+truncateOutput(obs.Error))
		}
		lines = append(lines, "- "+strings.Join(parts, " "))
	}
	return strings.Join(lines, "\n")
}

func guardFinalResponse(response string, result AgentCommandLoopResult) string {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		trimmed = strings.TrimSpace(result.Summary)
	}
	if !hasObservedOutput(result.Transcript) && responseClaimsSpecificFact(trimmed) {
		return "No command output was captured, so I cannot verify the requested fact from tool evidence."
	}

	guards := make([]string, 0, 3)
	if result.BlockedCount > 0 {
		if obs, ok := firstObservationWithStatus(result.Transcript, "blocked"); ok {
			guards = append(guards, "Blocked: "+formatObservationGuard(obs)+".")
		}
	}
	if result.FailedCount > 0 {
		if obs, ok := firstObservationWithStatus(result.Transcript, "failed"); ok {
			guards = append(guards, "Failed: "+formatObservationGuard(obs)+".")
		}
	}
	if result.Done {
		if path := firstSuccessfulMkdirPath(result.Transcript); path != "" {
			guards = append(guards, "Created: "+path+".")
		}
	}
	if len(guards) == 0 {
		return buildFinalRecap(trimmed, result)
	}
	if trimmed == "" {
		return buildFinalRecap(strings.Join(guards, "\n"), result)
	}
	return buildFinalRecap(strings.Join(append(guards, trimmed), "\n"), result)
}

func buildFinalRecap(response string, result AgentCommandLoopResult) string {
	return strings.Join([]string{
		"Tried: " + summarizeObservedCommands(result.Transcript, ""),
		"Did not work: " + summarizeObservedCommands(result.Transcript, "failed", "blocked"),
		"Worked: " + summarizeObservedCommands(result.Transcript, "success"),
		"Final: " + strings.TrimSpace(response),
	}, "\n")
}

func summarizeObservedCommands(transcript []CommandObservation, statuses ...string) string {
	wantAny := len(statuses) == 0 || statuses[0] == ""
	wanted := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		if status != "" {
			wanted[status] = struct{}{}
		}
	}

	items := make([]string, 0, 3)
	for _, obs := range transcript {
		if !wantAny {
			if _, ok := wanted[obs.Status]; !ok {
				continue
			}
		}
		item := summarizeObservation(obs)
		if item == "" {
			continue
		}
		items = append(items, item)
		if len(items) >= 3 {
			break
		}
	}
	if len(items) == 0 {
		return "none observed"
	}
	return strings.Join(items, "; ")
}

func summarizeObservation(obs CommandObservation) string {
	if strings.TrimSpace(obs.Command) != "" {
		if strings.TrimSpace(obs.Stdout) != "" {
			return obs.Command + " -> " + truncateOutput(obs.Stdout)
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			return obs.Command + " -> stderr: " + truncateOutput(obs.Stderr)
		}
		if strings.TrimSpace(obs.Error) != "" {
			return obs.Command + " -> " + truncateOutput(obs.Error)
		}
		return obs.Command + " -> " + obs.Status
	}
	if strings.TrimSpace(obs.Error) != "" {
		return truncateOutput(obs.Error)
	}
	if strings.TrimSpace(obs.Stdout) != "" {
		return truncateOutput(obs.Stdout)
	}
	return ""
}

func responseClaimsSpecificFact(response string) bool {
	fields := strings.Fields(response)
	for _, field := range fields {
		token := strings.Trim(field, ".,;:()[]{}\"'")
		if looksLikeYear(token) || looksLikeClockTime(token) || tokenContainsRune(token, 'T') && looksLikeYear(token[:minInt(len(token), 4)]) {
			return true
		}
	}
	return false
}

func looksLikeYear(token string) bool {
	if len(token) < 4 {
		return false
	}
	year := token[:4]
	if year[0] != '2' {
		return false
	}
	for _, r := range year {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func looksLikeClockTime(token string) bool {
	parts := strings.Split(token, ":")
	if len(parts) < 2 {
		return false
	}
	return allDigitsLen(parts[0], 1, 2) && allDigitsLen(parts[1], 2, 2)
}

func allDigitsLen(value string, minLen, maxLen int) bool {
	if len(value) < minLen || len(value) > maxLen {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstObservationWithStatus(transcript []CommandObservation, status string) (CommandObservation, bool) {
	for _, obs := range transcript {
		if obs.Status == status {
			return obs, true
		}
	}
	return CommandObservation{}, false
}

func formatObservationGuard(obs CommandObservation) string {
	if strings.TrimSpace(obs.Command) == "" {
		return truncateOutput(obs.Error)
	}
	if strings.TrimSpace(obs.Error) == "" {
		return obs.Command
	}
	return obs.Command + " (" + truncateOutput(obs.Error) + ")"
}

func firstSuccessfulMkdirPath(transcript []CommandObservation) string {
	for _, obs := range transcript {
		if obs.Status != "success" {
			continue
		}
		parts := strings.Fields(obs.Command)
		if len(parts) == 0 || parts[0] != "mkdir" {
			continue
		}
		for _, part := range parts[1:] {
			clean := cleanCommandPathToken(part)
			if clean == "" || strings.HasPrefix(clean, "-") {
				continue
			}
			return clean
		}
	}
	return ""
}

func tokenContainsRune(token string, target rune) bool {
	for _, r := range token {
		if r == target {
			return true
		}
	}
	return false
}

func finalResponseLooksOffTask(userInput, response, evidence string) bool {
	userTokens := finalReviewSignificantTokens(userInput)
	if len(userTokens) < 4 || len(response) <= 160 {
		return false
	}
	responseTokens := finalReviewTokenSet(response + "\n" + evidence)
	if len(responseTokens) == 0 {
		return true
	}
	overlap := 0
	for _, token := range userTokens {
		if _, ok := responseTokens[token]; ok {
			overlap++
		}
	}
	if overlap == 0 {
		return true
	}
	return (overlap*100)/len(userTokens) < 8
}

func finalReviewSignificantTokens(value string) []string {
	matches := finalReviewTokenPattern.FindAllString(strings.ToLower(value), -1)
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, token := range matches {
		if finalReviewStopword(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func finalReviewTokenSet(value string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, token := range finalReviewSignificantTokens(value) {
		set[token] = struct{}{}
	}
	return set
}

func finalReviewStopword(token string) bool {
	switch token {
	case "the", "and", "for", "that", "this", "with", "you", "your", "are", "was", "were", "have", "has", "had", "from", "into", "what", "when", "where", "which", "who", "why", "how", "can", "could", "would", "should", "about", "please", "need", "want", "assistant", "response":
		return true
	default:
		return false
	}
}

func finalRequestLikelyNeedsTools(userInput string) bool {
	lower := strings.ToLower(userInput)
	for _, phrase := range []string{
		"current", "right now", "today", "latest", "weather", "time", "date", "search", "web", "internet", "browse", "file", "run", "create", "edit", "test", "build",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func buildFinalReviewCorrection(userInput, response, evidence string) string {
	lines := []string{
		"Self-review flagged the draft as weakly aligned with the current request.",
		"Request: " + truncateOutput(userInput),
	}
	if strings.TrimSpace(evidence) != "" {
		lines = append(lines, "Evidence: "+truncateOutput(evidence))
	}
	lines = append(lines, "Draft: "+truncateOutput(response))
	return strings.Join(lines, "\n")
}
