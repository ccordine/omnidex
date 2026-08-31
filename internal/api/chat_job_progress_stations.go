package api

import (
	"fmt"
	"strings"
)

func summarizeChatCodingStage(event parsedChatStepEvent) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(event.Message, "attempt", "generated_blocks")
	if err != nil {
		return "", "", err
	}
	attempt, err := requireChatEventInteger(fields, "attempt", false)
	if err != nil {
		return "", "", err
	}
	blocks, err := requireChatEventInteger(fields, "generated_blocks", false)
	if err != nil {
		return "", "", err
	}
	if event.Type == "coding_stage_passed" {
		return chatProgressVerification, fmt.Sprintf("Staged validation passed for %d typed blocks", blocks), nil
	}
	return chatProgressVerification, fmt.Sprintf("Staged validation attempt %d for %d typed blocks", attempt, blocks), nil
}

func summarizeChatCodingCorrection(message string) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(message, "block", "guidance_bytes")
	if err != nil {
		return "", "", err
	}
	block, err := requireChatEventToken(fields, "block", 256)
	if err != nil {
		return "", "", err
	}
	guidanceBytes, err := requireChatEventInteger(fields, "guidance_bytes", false)
	return chatProgressDiagnostic, fmt.Sprintf(
		"Applying a %d-byte repair instruction to %s", guidanceBytes, block,
	), err
}

func summarizeChatCodingRepairGuidance(message string) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(message, "block", "exact_failure")
	if err != nil {
		return "", "", err
	}
	block, err := requireChatEventToken(fields, "block", 256)
	if err != nil {
		return "", "", err
	}
	failure, err := requireChatEventText(fields, "exact_failure", maxChatProgressRawBytes)
	return chatProgressDiagnostic, fmt.Sprintf(
		"Analyzing a repair for %s: %s", block, boundedChatProgressText(failure),
	), err
}

func chatPortableEventIdentity(eventType string) (namespace, state string, ok bool) {
	for _, candidate := range []string{"coding", "objective", "web_research"} {
		prefix := candidate + "_"
		if !strings.HasPrefix(eventType, prefix) {
			continue
		}
		state = strings.TrimPrefix(eventType, prefix)
		switch state {
		case "portable_dispatched", "worker_started", "worker_rejected", "worker_completed", "worker_failed":
			return candidate, state, true
		}
	}
	return "", "", false
}

func summarizeChatPortableEvent(namespace, state, message string) (chatProgressKind, string, error) {
	if state == "portable_dispatched" {
		fields, err := exactChatEventFields(message, "kind", "work", "payload", "model")
		if err != nil {
			return "", "", err
		}
		kind, err := requireChatEventToken(fields, "kind", 128)
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventToken(fields, "work", 64); err != nil {
			return "", "", err
		}
		bytes, err := parseChatEventByteCount(fields["payload"])
		if err != nil {
			return "", "", err
		}
		label, category := chatStationLabel(kind)
		return category, fmt.Sprintf("Dispatched %s station (%d-byte envelope)", label, bytes), nil
	}
	keys := []string{"kind", "subject", "model", "attempt"}
	if state == "worker_started" {
		keys = append(keys, "context")
	} else if state == "worker_rejected" || state == "worker_failed" {
		keys = append(keys, "error")
	}
	fields, err := exactChatEventFields(message, keys...)
	if err != nil {
		if state != "worker_started" {
			return "", "", err
		}
		fields, err = exactChatEventFields(message, "kind", "subject", "model", "attempt")
		if err != nil {
			return "", "", err
		}
	}
	subject, err := requireChatEventToken(fields, "subject", 256)
	if err != nil {
		return "", "", err
	}
	attempt, err := requireChatEventAttempt(fields)
	if err != nil {
		return "", "", err
	}
	label, category := chatStationLabel(subject)
	verb := map[string]string{
		"worker_started": "started", "worker_rejected": "rejected",
		"worker_completed": "completed", "worker_failed": "failed",
	}[state]
	summary := fmt.Sprintf("%s station %s (attempt %s)", label, verb, attempt)
	if state == "worker_rejected" || state == "worker_failed" {
		detail, detailErr := requireChatEventText(fields, "error", maxChatProgressRawBytes)
		if detailErr != nil {
			return "", "", detailErr
		}
		summary += ": " + boundedChatProgressText(detail)
		category = chatProgressDiagnostic
	}
	_ = namespace
	return category, summary, nil
}

func chatStationLabel(subject string) (string, chatProgressKind) {
	labels := map[string]string{
		"context_relevance":             "Context relevance",
		"context_minification":          "Context minification",
		"conversation_objective_kind":   "Objective classification",
		"conversation_response":         "Response",
		"grounded_answer":               "Grounded answer",
		"database_schema_selection":     "Database schema selection",
		"database_query_intent":         "Database relational intent",
		"database_join_path_selection":  "Database relationship selection",
		"repository_change_surface":     "Repository change-surface",
		"web_relevance":                 "Web relevance",
		"web_grounded_synthesis":        "Web grounded synthesis",
	}
	label := labels[subject]
	if label == "" {
		label = displayChatProgressToken(subject)
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	category := chatProgressStation
	switch subject {
	case "context_relevance", "context_minification",
		"database_schema_selection",
		"database_join_path_selection",
		"web_relevance":
		category = chatProgressRetrieval
	}
	return label, category
}
