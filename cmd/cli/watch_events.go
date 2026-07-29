package main

import (
	"net/url"
	"regexp"
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
	case "plan_begin":
		return "Inspecting request and drafting plan"
	case "plan_ready":
		return "Plan ready"
	case "tooling_begin":
		return "Inspecting environment and required tools"
	case "tooling_ready":
		return "Tooling check complete"
	case "workspace_scan_begin":
		return "Exploring workspace files"
	case "workspace_scan_ready":
		return "Workspace scan complete"
	case "tag_begin":
		return "Tagging instruction context"
	case "tag_ready":
		return "Tags ready"
	case "retrieve_begin":
		return "Retrieving relevant memory"
	case "retrieve_ready":
		return "Memory retrieval complete"
	case "analyze_begin":
		return "Analyzing gathered context"
	case "analyze_ready":
		return "Analysis complete"
	case "response_begin":
		return "Drafting response"
	case "response_ready":
		return "Response draft ready"
	case "verify_begin":
		return "Reviewing and verifying response"
	case "verify_ready":
		return "Verification complete"
	case "web_search_begin":
		return "Exploring web sources"
	case "web_search_ready":
		return "Web research context ready"
	case "web_search_skipped":
		reason := eventMessageField(message, "reason")
		if reason != "" {
			return "Web search skipped (" + reason + ")"
		}
		return "Web search skipped"
	case "llm_stream_progress":
		outputBytes := eventMessageField(message, "output_bytes")
		elapsed := eventMessageField(message, "elapsed")
		if outputBytes == "" || elapsed == "" {
			return "Generating model response"
		}
		return "Generating model response (" + outputBytes + " bytes, " + elapsed + " elapsed)"
	case "coding_phase_changed":
		return summarizeCodingPhase(message)
	case "coding_assembly_ready":
		files := eventMessageField(message, "files")
		if files == "" {
			return "Deterministic assembly ready"
		}
		return "Deterministic assembly ready: " + files + " source units"
	case "coding_file_started":
		path := eventMessageField(message, "path")
		stage := eventMessageField(message, "stage")
		if stage == "repair" {
			return "Repairing " + fallbackWatchValue(path, "assigned file")
		}
		return "Generating " + fallbackWatchValue(path, "assigned file")
	case "coding_file_written":
		path := fallbackWatchValue(eventMessageField(message, "path"), "assigned file")
		bytes := eventMessageField(message, "bytes")
		if bytes != "" {
			return "Accepted " + path + " (" + bytes + " bytes)"
		}
		return "Accepted " + path
	case "coding_file_unchanged":
		return "File station made no change to " + fallbackWatchValue(eventMessageField(message, "path"), "the assigned file")
	case "coding_verification_failed":
		return "Verification failed: " + fallbackWatchValue(eventMessageField(message, "command"), "see diagnostic")
	case "coding_static_validation_failed":
		return "Static validation failed: " + fallbackWatchValue(eventMessageTail(message, "diagnostic"), "see diagnostic")
	case "coding_repair_selected":
		repair := fallbackWatchValue(eventMessageField(message, "repair"), "?")
		path := fallbackWatchValue(eventMessageField(message, "path"), "assigned file")
		return "Selected " + path + " for diagnostic repair " + repair
	case "coding_fragment_correction_started":
		block := fallbackWatchValue(eventMessageField(message, "block"), "assigned function")
		failure := fallbackWatchValue(eventMessageTail(message, "exact_failure"), "see diagnostic")
		return "Correcting " + block + ": " + compactProgressValue(failure, 240)
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
	case "coding_completed":
		return "Coding workflow completed with verified workspace"
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
	case "llm_stream_progress",
		"coding_phase_changed",
		"coding_assembly_ready",
		"coding_file_started",
		"coding_file_written",
		"coding_file_deleted",
		"coding_file_unchanged",
		"coding_verification_failed",
		"coding_static_validation_failed",
		"coding_repair_selected",
		"coding_fragment_correction_started",
		"coding_worker_rejected",
		"coding_worker_failed",
		"coding_completed":
		return true
	default:
		return false
	}
}

func codingWorkerLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "plan":
		return "Plan"
	case "repair":
		return "Repair"
	case "file":
		return "File"
	default:
		return "Typed"
	}
}

func summarizeCodingPhase(message string) string {
	switch strings.ToLower(eventMessageField(message, "phase")) {
	case "planning":
		return "Compiling a bounded construction plan"
	case "constructing":
		return "Constructing planned files"
	case "verifying":
		return "Verifying accepted workspace"
	case "repairing":
		return "Repairing exact diagnostic"
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
	case strings.HasPrefix(lower, "web search query:"):
		query := strings.TrimSpace(value[len("web search query:"):])
		return "Explore", compactProgressValue("Search web for "+query, maxChars)
	case strings.HasPrefix(lower, "web search context chars="):
		return "Explore", compactProgressValue("Collected web research context", maxChars)
	case strings.HasPrefix(lower, "tool check:"):
		check := strings.TrimSpace(value[len("tool check:"):])
		return "Inspect", compactProgressValue(check, maxChars)
	case strings.HasPrefix(lower, "running test:"):
		command := strings.TrimSpace(value[len("running test:"):])
		return "Run", compactProgressValue("Test "+command, maxChars)
	case strings.HasPrefix(lower, "plan generated chars="):
		return "Plan", compactProgressValue("Generated planning draft", maxChars)
	}

	if strings.EqualFold(stream, "stderr") {
		return "Warn", compactProgressValue(value, maxChars)
	}
	return "Inspect", compactProgressValue(value, maxChars)
}

func summarizeWebSearchDomains(domains []string, maxChars int) string {
	if len(domains) == 0 {
		return ""
	}
	return compactProgressValue(strings.Join(domains, ", "), maxChars)
}

func webSearchDomainsFromContext(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	matches := urlPattern.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, raw := range matches {
		clean := strings.TrimSpace(strings.TrimRight(raw, ".,);]}>\"'"))
		if clean == "" {
			continue
		}
		parsed, err := url.Parse(clean)
		if err != nil {
			continue
		}
		host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out
}
