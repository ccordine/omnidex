package main

import (
	"strings"
)

type stepEventPayload struct {
	Time      string
	EventType string
	Message   string
}

func parseStepEventPayload(raw string) stepEventPayload {
	fields := strings.Fields(strings.TrimSpace(raw))
	payload := stepEventPayload{}
	rest := make([]string, 0, len(fields))
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "time="):
			payload.Time = strings.TrimSpace(strings.TrimPrefix(field, "time="))
		case strings.HasPrefix(field, "event="):
			payload.EventType = strings.TrimSpace(strings.TrimPrefix(field, "event="))
		default:
			rest = append(rest, field)
		}
	}
	payload.Message = strings.TrimSpace(strings.Join(rest, " "))
	return payload
}

func summarizeStepEvent(payload stepEventPayload) string {
	eventType := strings.ToLower(strings.TrimSpace(payload.EventType))
	message := strings.TrimSpace(payload.Message)
	switch eventType {
	case "step_start":
		return "Starting step execution"
	case "step_complete":
		return "Step completed"
	case "coding_phase_changed":
		return summarizeCodingPhase(message)
	case "coding_assembly_ready":
		files := eventMessageField(message, "files")
		if files == "" {
			return "Deterministic assembly ready"
		}
		return "Deterministic assembly ready: " + files + " source units"
	case "coding_stage_started":
		return "Running isolated staged validation"
	case "coding_stage_passed":
		return "Isolated staged validation passed"
	case "coding_target_tree_validation_failed":
		return "Target tree validation failed: " + fallbackWatchValue(eventMessageTail(message, "diagnostic"), "see diagnostic")
	case "coding_compiler_repair_applied":
		return "Applied deterministic compiler repair to " + fallbackWatchValue(eventMessageField(message, "block"), "assigned block")
	case "coding_portable_correction_dispatched":
		work := fallbackWatchValue(eventMessageField(message, "work"), "assigned source job")
		iteration := fallbackWatchValue(eventMessageField(message, "iteration"), "unknown")
		return "Continuing source job " + work + " in the same model context (iteration " + iteration + ")"
	case "coding_worker_rejected":
		kind := codingWorkerLabel(eventMessageField(message, "kind"))
		subject := fallbackWatchValue(eventMessageField(message, "subject"), "assigned input")
		attempt := eventMessageField(message, "attempt")
		reason := eventMessageTail(message, "error")
		summary := kind + " station rejected " + subject
		if attempt != "" {
			summary += " (" + attempt + ")"
		}
		if reason != "" {
			summary += ": " + compactProgressValue(reason, 240)
		}
		return summary
	case "coding_worker_failed":
		kind := codingWorkerLabel(eventMessageField(message, "kind"))
		subject := fallbackWatchValue(eventMessageField(message, "subject"), "assigned input")
		reason := eventMessageTail(message, "error")
		summary := kind + " station failed for " + subject
		if reason != "" {
			summary += ": " + compactProgressValue(reason, 240)
		}
		return summary
	}

	if strings.HasSuffix(eventType, "_waiting_input") {
		if message != "" {
			return "Waiting for your input (" + message + ")"
		}
		return "Waiting for your input"
	}
	if strings.HasSuffix(eventType, "_error") {
		if message != "" {
			return "Step error: " + message
		}
		return "Step error"
	}
	if message != "" {
		return strings.TrimSpace(eventType + " " + message)
	}
	if eventType != "" {
		return eventType
	}
	return "event update"
}

func showStepEventInSlimProgress(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "coding_phase_changed",
		"coding_assembly_ready",
		"coding_stage_started",
		"coding_stage_passed",
		"coding_target_tree_validation_failed",
		"coding_compiler_repair_applied",
		"coding_portable_correction_dispatched",
		"coding_worker_rejected",
		"coding_worker_failed":
		return true
	default:
		return false
	}
}

func codingWorkerLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "semantic":
		return "Semantic"
	case "fragment":
		return "Source"
	default:
		return "Typed"
	}
}

func summarizeCodingPhase(message string) string {
	switch strings.ToLower(eventMessageField(message, "phase")) {
	case "assembling":
		return "Compiling deterministic source assembly"
	case "constructing":
		return "Constructing accepted files"
	case "verifying":
		return "Verifying accepted workspace"
	case "deploying":
		return "Deploying the verified workload"
	case "completed":
		return "Coding workflow completed"
	case "failed":
		return "Coding workflow failed"
	default:
		return "Coding workflow state changed"
	}
}

func fallbackWatchValue(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func eventMessageField(message, key string) string {
	needle := strings.TrimSpace(key) + "="
	for _, field := range strings.Fields(strings.TrimSpace(message)) {
		if !strings.HasPrefix(field, needle) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(field, needle))
	}
	return ""
}

func eventMessageTail(message, key string) string {
	needle := strings.TrimSpace(key) + "="
	message = strings.TrimSpace(message)
	index := strings.Index(message, needle)
	if index < 0 {
		return ""
	}
	if index > 0 && message[index-1] != ' ' {
		return ""
	}
	return strings.TrimSpace(message[index+len(needle):])
}

func summarizeProgressStream(stream, value string, maxChars int) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(lower, "running test:"):
		command := strings.TrimSpace(value[len("running test:"):])
		return "Run", compactProgressValue("Test "+command, maxChars)
	}

	if strings.EqualFold(stream, "stderr") {
		return "Warn", compactProgressValue(value, maxChars)
	}
	return "Inspect", compactProgressValue(value, maxChars)
}
