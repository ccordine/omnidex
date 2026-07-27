package omni

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func formatStructuredCommandChatResponse(result CommandDecisionResult, stdout, stderr, errText string) string {
	partialStopped := result.PartialProgress && strings.TrimSpace(errText) != ""
	statusLabel := "Exit code"
	if partialStopped {
		statusLabel = "Last command exit code"
	}
	lines := []string{}
	if partialStopped {
		lines = append(lines, "Partial result")
		lines = append(lines, "--------------")
		lines = append(lines, "Outcome: partial progress only; completion was not accepted.")
		lines = append(lines, "Completion: not accepted")
	} else {
		lines = append(lines, "Result")
		lines = append(lines, "------")
		if strings.TrimSpace(errText) == "" && result.ExitCode == 0 {
			lines = append(lines, "Outcome: complete; completion was accepted.")
		} else if strings.TrimSpace(errText) != "" {
			lines = append(lines, "Outcome: failed; no completion was accepted.")
		}
	}
	if strings.TrimSpace(result.Command) != "" {
		commandLabel := "Command"
		if partialStopped {
			commandLabel = "Last attempted command"
		}
		lines = appendFormattedResponseValue(lines, commandLabel, result.Command)
	} else if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Command: (none accepted)")
	}
	lines = append(lines, fmt.Sprintf("%s: %d", statusLabel, result.ExitCode))
	if len(result.Observations) > 1 {
		lines = append(lines, fmt.Sprintf("Attempts: %d", len(result.Observations)))
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "")
		stdoutLabel := "Stdout"
		if partialStopped {
			stdoutLabel = "Latest captured stdout"
		}
		lines = appendFormattedResponseValue(lines, stdoutLabel, truncateOutput(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "")
		stderrLabel := "Stderr"
		if partialStopped {
			stderrLabel = "Latest captured stderr"
		}
		lines = appendFormattedResponseValue(lines, stderrLabel, truncateOutput(stderr))
	}
	if strings.TrimSpace(result.Answer) != "" {
		lines = append(lines, "")
		answerLabel := "Answer"
		if partialStopped {
			answerLabel = "Latest captured answer"
		}
		lines = appendFormattedResponseValue(lines, answerLabel, result.Answer)
	}
	blocker := latestStructuredFailureSummary(result.Observations)
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "", "Status:")
		if result.PartialProgress {
			if pending := pendingStructuredObjectiveIDs(result.ObjectiveLedger); pending != "" {
				lines = append(lines, "  Pending objectives: "+pending)
			}
			if blocker != "" {
				label := "Last blocker"
				if strings.Contains(blocker, "anti_loop:") {
					label = "Loop blocker"
				}
				lines = append(lines, "  "+label+": "+blocker)
			}
			lines = append(lines, "  Stopped: "+errText)
		} else {
			if blocker != "" {
				label := "Last blocker"
				if strings.Contains(blocker, "anti_loop:") {
					label = "Loop blocker"
				}
				lines = append(lines, "  "+label+": "+blocker)
			}
			lines = append(lines, "  Error: "+errText)
		}
		if diagnosis := classifyStructuredLLMFailure(errors.New(errText)); diagnosis != "ollama_request_failure" {
			lines = append(lines, "  Diagnosis: "+diagnosis)
		}
	} else if !result.PartialProgress && result.ExitCode == 0 {
		lines = appendStructuredCompletionRecap(lines, result)
	}
	return strings.Join(lines, "\n")
}

func appendStructuredCompletionRecap(lines []string, result CommandDecisionResult) []string {
	recap := structuredCompletionRecapLines(result)
	if len(recap) == 0 {
		return lines
	}
	lines = append(lines, "", "Recap:")
	for _, line := range recap {
		lines = append(lines, "  "+line)
	}
	return lines
}

func structuredCompletionRecapLines(result CommandDecisionResult) []string {
	lines := []string{}
	if result.Elapsed > 0 {
		lines = append(lines, "Elapsed: "+formatStructuredElapsed(result.Elapsed))
	}
	if completed := completedStructuredObjectiveIDs(result.ObjectiveLedger); completed != "" {
		lines = append(lines, "Completed objectives: "+completed)
	}
	if evidence := structuredCompletionEvidenceSummary(result.ObjectiveLedger, result.Observations, 4); evidence != "" {
		lines = append(lines, "Evidence accepted: "+evidence)
	}
	if actions := structuredSuccessfulActionSummary(result.Observations, 4); actions != "" {
		lines = append(lines, "Actions: "+actions)
	}
	if decisions := structuredDecisionSummary(result.ObjectiveLedger, result.Observations, 4); decisions != "" {
		lines = append(lines, "Decisions: "+decisions)
	}
	return lines
}

func completedStructuredObjectiveIDs(ledger []StructuredObjective) string {
	ids := []string{}
	for _, objective := range ledger {
		if structuredObjectiveSatisfied(objective) {
			id := strings.TrimSpace(objective.ID)
			if id == "" {
				id = strings.TrimSpace(objective.Description)
			}
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return strings.Join(ids, ",")
}

func structuredCompletionEvidenceSummary(ledger []StructuredObjective, observations []StructuredCommandObservation, limit int) string {
	items := []string{}
	for _, objective := range ledger {
		if !structuredObjectiveSatisfied(objective) {
			continue
		}
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			id = strings.TrimSpace(objective.Description)
		}
		evidence := strings.TrimSpace(objective.Evidence)
		if id == "" || evidence == "" {
			continue
		}
		items = append(items, truncateStructuredRecapItem(id+"="+evidence))
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		for _, obs := range observations {
			if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
				continue
			}
			evidence := strings.TrimSpace(firstNonEmpty(obs.Stdout, obs.Stderr, obs.Command))
			if evidence == "" {
				continue
			}
			items = append(items, truncateStructuredRecapItem(evidence))
			if len(items) >= limit {
				break
			}
		}
	}
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, "; ")
}

func structuredSuccessfulActionSummary(observations []StructuredCommandObservation, limit int) string {
	actions := []string{}
	seen := map[string]bool{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if command == "" || obs.ExitCode != 0 {
			continue
		}
		key := normalizeStructuredCommandForComparison(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, truncateStructuredRecapItem(command))
		if len(actions) >= limit {
			break
		}
	}
	if len(actions) == 0 {
		return ""
	}
	if more := countAdditionalSuccessfulActions(observations, seen); more > 0 {
		actions = append(actions, fmt.Sprintf("+%d more", more))
	}
	return strings.Join(actions, "; ")
}

func countAdditionalSuccessfulActions(observations []StructuredCommandObservation, seen map[string]bool) int {
	count := 0
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if command == "" || obs.ExitCode != 0 {
			continue
		}
		key := normalizeStructuredCommandForComparison(command)
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
}

func structuredDecisionSummary(ledger []StructuredObjective, observations []StructuredCommandObservation, limit int) string {
	decisions := []string{}
	for _, objective := range ledger {
		if objective.Source == structuredObjectiveSourceEvidenceRequiredPrerequisite {
			description := strings.TrimSpace(objective.ID)
			if description == "" {
				description = strings.TrimSpace(objective.Description)
			}
			if description != "" {
				decisions = appendUniqueRecapDecision(decisions, "added evidence-required prerequisite "+description, limit)
			}
		}
	}
	for _, obs := range observations {
		if obs.Cached {
			decisions = appendUniqueRecapDecision(decisions, "reused cached command evidence", limit)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			decisions = appendUniqueRecapDecision(decisions, "rejected proposed command "+structuredCommandNameForRecap(obs.RejectedCommand), limit)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			decisions = appendUniqueRecapDecision(decisions, "revised after evaluator feedback", limit)
		}
		if strings.TrimSpace(obs.Question) != "" {
			decisions = appendUniqueRecapDecision(decisions, "used user input for "+truncateStructuredRecapItem(obs.Question), limit)
		}
		if len(decisions) >= limit {
			break
		}
	}
	return strings.Join(decisions, "; ")
}

func appendUniqueRecapDecision(decisions []string, decision string, limit int) []string {
	if strings.TrimSpace(decision) == "" || len(decisions) >= limit {
		return decisions
	}
	for _, existing := range decisions {
		if existing == decision {
			return decisions
		}
	}
	return append(decisions, decision)
}

func truncateStructuredRecapItem(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	const max = 96
	if len(value) <= max {
		return value
	}
	return value[:max-3] + "..."
}

func structuredCommandNameForRecap(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if len(name) > 48 {
		name = name[:45] + "..."
	}
	return name
}

func formatStructuredElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return fmt.Sprintf("%dms", elapsed.Milliseconds())
	}
	return elapsed.Round(100 * time.Millisecond).String()
}

func appendFormattedResponseValue(lines []string, label, value string) []string {
	value = strings.TrimRight(value, "\n")
	if strings.Contains(value, "\n") {
		parts := strings.Split(value, "\n")
		lines = append(lines, label+": "+parts[0])
		if len(parts) > 1 {
			lines = append(lines, indentTimelineBlock(strings.Join(parts[1:], "\n"), "  "))
		}
		return lines
	}
	return append(lines, label+": "+value)
}

func structuredCommandResponseStreams(result CommandDecisionResult, stdout, stderr string, execErr error) (string, string) {
	if latest, ok := latestCommandObservationForResponse(result.Observations, result.Command); ok {
		return latest.Stdout, latest.Stderr
	}
	if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok {
		return latest.Stdout, latest.Stderr
	}
	return stdout, stderr
}

func latestCommandObservationForResponse(observations []StructuredCommandObservation, command string) (StructuredCommandObservation, bool) {
	if strings.TrimSpace(command) != "" {
		normalized := normalizeStructuredCommandForComparison(command)
		for i := len(observations) - 1; i >= 0; i-- {
			if normalizeStructuredCommandForComparison(observations[i].Command) == normalized {
				return observations[i], true
			}
		}
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) != "" {
			return observations[i], true
		}
	}
	return StructuredCommandObservation{}, false
}

func latestStructuredFailureSummary(observations []StructuredCommandObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			return truncateOutput(obs.Stderr)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			return truncateOutput(obs.EvaluationFeedback)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			return "rejected command: " + truncateOutput(obs.RejectedCommand)
		}
	}
	return ""
}

func (a *App) reviewFinalResponse(ctx context.Context, userInput, response string, evidence []string, emitEvent func(string, string, map[string]string)) string {
	review := ReviewFinalAssistantResponse(FinalAssistantResponseReviewInput{
		UserInput: userInput,
		Response:  response,
		Evidence:  evidence,
	})
	finalResponse := review.Response
	details := map[string]string{
		"passed":     fmt.Sprintf("%t", review.Passed),
		"confidence": fmt.Sprintf("%d", review.Confidence),
		"feedback":   truncateOutput(review.Feedback),
	}

	if a.evaluator != nil {
		evaluation, err := a.evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
			Step:        0,
			UserPrompt:  userInput,
			PlannerJob:  finalResponseReviewerJobSummary(),
			LLMResponse: finalResponse,
			Observations: []StructuredCommandObservation{{
				Step:     0,
				Command:  "FINAL_RESPONSE_EVIDENCE",
				ExitCode: 0,
				Stdout:   truncateStructuredObservation(strings.Join(evidence, "\n")),
			}},
		})
		if err != nil {
			details["evaluator_error"] = truncateOutput(err.Error())
		} else if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
			details["evaluator_error"] = truncateOutput(consistencyErr.Error())
			details["evaluator_confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
			details["evaluator_feedback"] = truncateOutput(evaluation.Feedback)
		} else {
			details["evaluator_confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
			details["evaluator_feedback"] = truncateOutput(evaluation.Feedback)
			if evaluation.Confidence < normalizeStructuredEvaluatorThreshold(a.evaluatorThreshold) {
				review.Passed = false
				review.Confidence = evaluation.Confidence
				review.Feedback = strings.TrimSpace(evaluation.Feedback)
				finalResponse = buildFinalReviewCorrection(userInput, finalResponse, strings.Join(evidence, "\n"))
				details["passed"] = "false"
				details["confidence"] = fmt.Sprintf("%d", evaluation.Confidence)
				details["feedback"] = truncateOutput(review.Feedback)
			}
		}
	}

	if emitEvent != nil {
		eventType := "final_response_review_passed"
		summary := "Final response self-review passed"
		if !review.Passed {
			eventType = "final_response_review_revised"
			summary = "Final response self-review revised response"
		}
		emitEvent(eventType, summary, details)
	}
	if a.runLogger != nil {
		_ = a.runLogger.Log("final_response_review", "completed", map[string]interface{}{
			"user_input": userInput,
			"response":   finalResponse,
			"details":    details,
		})
	}
	return finalResponse
}

func finalResponseReviewerJobSummary() string {
	return strings.Join([]string{
		"Review the final user-facing assistant response before it is shown.",
		"Score whether it directly answers the current user prompt, stays grounded in provided evidence, and does not drift to prior tasks.",
		"High confidence means it is on task and safe to send.",
		"Low confidence means it is empty, off-task, overclaims, ignores evidence, or falsely refuses available tool capability.",
	}, " ")
}

func structuredResponseReviewEvidence(result CommandDecisionResult, stdout, stderr string, execErr error) []string {
	evidence := []string{
		"command=" + result.Command,
		fmt.Sprintf("exit_code=%d", result.ExitCode),
	}
	if strings.TrimSpace(stdout) != "" {
		evidence = append(evidence, "stdout="+stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		evidence = append(evidence, "stderr="+stderr)
	}
	if strings.TrimSpace(result.Answer) != "" {
		evidence = append(evidence, "answer="+result.Answer)
	}
	if execErr != nil {
		evidence = append(evidence, "error="+execErr.Error())
	}
	return evidence
}
