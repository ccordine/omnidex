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
		"Correcting %s: %s", block, boundedChatProgressText(failure),
	), err
}

func summarizeChatSkillBinding(message string) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(message, "requirement", "skill", "version", "source", "status")
	if err != nil {
		return "", "", err
	}
	requirement, err := requireChatEventText(fields, "requirement", 256)
	if err != nil {
		return "", "", err
	}
	skill, err := requireChatEventText(fields, "skill", 256)
	if err != nil {
		return "", "", err
	}
	version, err := requireChatEventInteger(fields, "version", false)
	if err != nil {
		return "", "", err
	}
	if _, err := requireChatEventToken(fields, "status", 32); err != nil {
		return "", "", err
	}
	return chatProgressRetrieval, fmt.Sprintf("Bound skill %s v%d to %s", skill, version, requirement), nil
}

func summarizeChatRepositoryIndex(event parsedChatStepEvent) (chatProgressKind, string, error) {
	switch event.Type {
	case "repository_index_started":
		fields, err := exactChatEventFields(event.Message, "authority")
		if err != nil || fields["authority"] != "server" {
			return "", "", firstChatProgressError(err, fmt.Errorf("repository index authority must be server"))
		}
		return chatProgressRetrieval, "Indexing the authoritative repository state", nil
	case "repository_index_failed":
		message, err := requireChatProgressMessage(event.Message)
		return chatProgressDiagnostic, "Repository indexing failed: " + message, err
	case "repository_index_ready":
		fields, err := exactChatEventFields(event.Message, "snapshot", "files", "analyses")
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "snapshot", 256); err != nil {
			return "", "", err
		}
		files, err := requireChatEventInteger(fields, "files", true)
		if err != nil {
			return "", "", err
		}
		analyses, err := requireChatEventInteger(fields, "analyses", true)
		return chatProgressRetrieval, fmt.Sprintf("Repository index ready: %d files, %d analyses", files, analyses), err
	}
	return "", "", fmt.Errorf("repository index event %q is not registered", event.Type)
}

func summarizeChatRepositoryChange(event parsedChatStepEvent) (chatProgressKind, string, error) {
	if event.Type == "repository_change_staged" || event.Type == "repository_desired_state_staged" {
		fields, err := exactChatEventFields(event.Message, "contract", "files")
		if event.Type == "repository_desired_state_staged" {
			fields, err = exactChatEventFields(event.Message, "graph", "files")
		}
		if err != nil {
			return "", "", err
		}
		files, err := requireChatEventInteger(fields, "files", false)
		return chatProgressReview, fmt.Sprintf("Staged an exact repository change across %d files", files), err
	}
	fields, err := exactChatEventFields(event.Message, "contract", "files", "snapshot")
	if event.Type == "repository_desired_state_verified" {
		fields, err = exactChatEventFields(event.Message, "graph", "files", "snapshot")
	}
	if err != nil {
		return "", "", err
	}
	files, err := requireChatEventInteger(fields, "files", false)
	if err != nil {
		return "", "", err
	}
	if _, err := requireChatEventText(fields, "snapshot", 256); err != nil {
		return "", "", err
	}
	return chatProgressFile, fmt.Sprintf("Committed an exact repository change across %d files", files), nil
}

func summarizeChatRepositoryVerification(event parsedChatStepEvent) (chatProgressKind, string, error) {
	if event.Type == "repository_verification_command_passed" {
		fields, err := exactChatEventFields(event.Message, "scope", "command")
		if err != nil {
			return "", "", err
		}
		command, err := requireChatEventText(fields, "command", 512)
		return chatProgressVerification, "Repository verification passed: " + command, err
	}
	fields, err := exactChatEventFields(event.Message, "scope", "plan")
	if err != nil {
		return "", "", err
	}
	if _, err := requireChatEventText(fields, "plan", 256); err != nil {
		return "", "", err
	}
	if event.Type == "repository_verification_baseline_accepted" {
		return chatProgressVerification, "Accepted the clean repository verification baseline", nil
	}
	return chatProgressVerification, "Accepted the code-owned repository verification plan", nil
}

func summarizeChatRepositoryRecovery(event parsedChatStepEvent) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(event.Message, "stage", "snapshot")
	if err != nil {
		return "", "", err
	}
	if _, err := requireChatEventText(fields, "stage", 256); err != nil {
		return "", "", err
	}
	if _, err := requireChatEventText(fields, "snapshot", 256); err != nil {
		return "", "", err
	}
	if event.Type == "repository_mutation_recovered" {
		return chatProgressFile, "Recovered and reconciled the durable repository mutation", nil
	}
	return chatProgressReview, "Reconciling a durable repository mutation", nil
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
		"conversation_context_selection":    "Context selection",
		"conversation_objective_kind":       "Objective classification",
		"conversation_response":             "Response",
		"grounded_answer":                   "Grounded answer",
		"repository_search_term":            "Repository search-term",
		"repository_change_surface":         "Repository change-surface",
		"repository_evidence_relevance":     "Repository relevance",
		"repository_grounded_review":        "Repository grounded review",
		"repository_grounded_correction":    "Repository grounded correction",
		"web_search_terms":                  "Web search-term",
		"web_relevance":                     "Web relevance",
		"web_grounded_synthesis":            "Web grounded synthesis",
		"web_grounded_synthesis_correction": "Web synthesis correction",
		"web_claim_evidence_review":         "Web claim review",
		"skill_selection":                   "Skill selection",
		"response_correction":               "Response correction",
	}
	label := labels[subject]
	if label == "" {
		label = displayChatProgressToken(subject)
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	category := chatProgressStation
	switch subject {
	case "conversation_context_selection", "repository_search_term", "repository_evidence_relevance",
		"web_search_terms", "web_relevance", "skill_selection":
		category = chatProgressRetrieval
	case "repository_grounded_review", "repository_grounded_correction", "web_claim_evidence_review", "response_correction":
		category = chatProgressReview
	}
	return label, category
}
