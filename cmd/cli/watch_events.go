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
