package api

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

func summarizeChatStepEvent(event parsedChatStepEvent, stepAction string) (chatProgressKind, string, error) {
	if kind, summary, matched, err := summarizeChatDeterministicCognitionEvent(event); matched {
		return kind, summary, err
	}
	switch event.Type {
	case "step_start":
		fields, err := exactChatEventFields(event.Message, "phase", "action", "worker")
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventToken(fields, "phase", 64); err != nil {
			return "", "", err
		}
		if _, err := requireChatEventToken(fields, "worker", 256); err != nil {
			return "", "", err
		}
		return chatProgressActivity, "Started " + displayChatProgressToken(stepAction), nil
	case "step_complete":
		if _, err := exactChatEventFields(event.Message, "action", "worker"); err != nil {
			return "", "", err
		}
		return chatProgressActivity, "Step completed: " + displayChatProgressToken(stepAction), nil
	case "step_canceled":
		if _, err := exactChatEventFields(event.Message, "action", "worker"); err != nil {
			return "", "", err
		}
		return chatProgressDiagnostic, "Step canceled: " + displayChatProgressToken(stepAction), nil
	case "step_error":
		message, err := requireChatProgressMessage(event.Message)
		return chatProgressDiagnostic, "Step failed: " + message, err
	case "operation_heartbeat":
		fields, err := exactChatEventFields(event.Message, "operation", "elapsed")
		if err != nil {
			return "", "", err
		}
		operation, err := requireChatEventText(fields, "operation", 256)
		if err != nil {
			return "", "", err
		}
		elapsed, err := requireChatEventText(fields, "elapsed", 32)
		return chatProgressActivity, "Working on " + operation + " (" + elapsed + ")", err
	case "coding_phase_changed":
		return summarizeChatCodingPhase(event.Message)
	case "coding_specification_accepted":
		fields, err := exactChatEventFields(event.Message, "surface", "requirements", "product_bytes")
		if err != nil {
			return "", "", err
		}
		requirements, err := requireChatEventInteger(fields, "requirements", false)
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventInteger(fields, "product_bytes", false); err != nil {
			return "", "", err
		}
		return chatProgressPreparation, fmt.Sprintf("Accepted %d exact requirements", requirements), nil
	case "coding_assembly_ready":
		fields, err := exactChatEventFields(event.Message, "adapter", "files", "blocks", "waves")
		if err != nil {
			return "", "", err
		}
		files, err := requireChatEventInteger(fields, "files", false)
		if err != nil {
			return "", "", err
		}
		blocks, err := requireChatEventInteger(fields, "blocks", false)
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventInteger(fields, "waves", false); err != nil {
			return "", "", err
		}
		return chatProgressPreparation, fmt.Sprintf("Assembly ready: %d files, %d typed blocks", files, blocks), nil
	case "coding_artifact_sieve_passed":
		fields, err := exactChatEventFields(event.Message, "stack", "files")
		if err != nil {
			return "", "", err
		}
		stack, err := requireChatEventToken(fields, "stack", 128)
		if err != nil {
			return "", "", err
		}
		files, err := requireChatEventInteger(fields, "files", false)
		return chatProgressVerification, fmt.Sprintf(
			"Artifact sieve passed for %s: %d files", displayChatProgressToken(stack), files,
		), err
	case "coding_workload_frozen":
		fields, err := exactChatEventFields(event.Message, "tasks", "sha256")
		if err != nil {
			return "", "", err
		}
		tasks, err := requireChatEventInteger(fields, "tasks", false)
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventToken(fields, "sha256", 64); err != nil {
			return "", "", err
		}
		return chatProgressPreparation, fmt.Sprintf("Frozen workload with %d concrete tasks", tasks), nil
	case "coding_stage_started", "coding_stage_passed":
		return summarizeChatCodingStage(event)
	case "coding_fragment_repair_guidance_started":
		return summarizeChatCodingRepairGuidance(event.Message)
	case "coding_fragment_correction_started":
		return summarizeChatCodingCorrection(event.Message)
	case "repository_snapshot_started", "repository_snapshot_ready", "repository_snapshot_failed",
		"repository_analysis_started", "repository_analysis_ready", "repository_analysis_failed":
		return summarizeChatRepositoryIntelligence(event)
	}
	if namespace, state, ok := chatPortableEventIdentity(event.Type); ok {
		return summarizeChatPortableEvent(namespace, state, event.Message)
	}
	return "", "", fmt.Errorf("event type %q is not registered for chat presentation", event.Type)
}

func summarizeChatCodingPhase(message string) (chatProgressKind, string, error) {
	fields, err := exactChatEventFields(message, "phase", "detail")
	if err != nil {
		return "", "", err
	}
	phase, err := requireChatEventToken(fields, "phase", 32)
	if err != nil {
		return "", "", err
	}
	summaries := map[string]string{
		"assembling":   "Compiling the deterministic assembly",
		"constructing": "Constructing accepted files",
		"verifying":    "Verifying the workspace",
		"deploying":    "Deploying the verified workload",
		"completed":    "Coding workflow reached verified completion",
		"failed":       "Coding workflow failed",
	}
	summary, exists := summaries[phase]
	if !exists {
		return "", "", fmt.Errorf("coding phase %q is not registered", phase)
	}
	kind := chatProgressActivity
	if phase == "verifying" || phase == "completed" {
		kind = chatProgressVerification
	} else if phase == "failed" {
		kind = chatProgressDiagnostic
	}
	return kind, summary, nil
}

func requireChatEventText(fields map[string]string, key string, maxBytes int) (string, error) {
	value := fields[key]
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxBytes ||
		!utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("event field %q must be bounded canonical text", key)
	}
	return value, nil
}

func requireChatProgressMessage(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("event requires one canonical message")
	}
	return boundedChatProgressText(value), nil
}

func displayChatProgressToken(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return "work"
	}
	return value
}

func firstChatProgressError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func parseChatEventByteCount(value string) (int, error) {
	if !strings.HasSuffix(value, "B") {
		return 0, fmt.Errorf("event byte count must end in B")
	}
	parsed, err := strconv.Atoi(strings.TrimSuffix(value, "B"))
	if err != nil || parsed < 0 || fmt.Sprintf("%dB", parsed) != value {
		return 0, fmt.Errorf("event byte count is not canonical")
	}
	return parsed, nil
}
