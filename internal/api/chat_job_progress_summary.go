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
		return chatProgressReview, fmt.Sprintf("Accepted %d exact requirements", requirements), nil
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
		return chatProgressReview, fmt.Sprintf("Assembly ready: %d files, %d typed blocks", files, blocks), nil
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
		return chatProgressReview, fmt.Sprintf("Frozen workload with %d concrete tasks", tasks), nil
	case "coding_task_verification_started":
		fields, err := exactChatEventFields(event.Message, "task", "requirement_bytes")
		if err != nil {
			return "", "", err
		}
		task, err := requireChatEventToken(fields, "task", 256)
		if err != nil {
			return "", "", err
		}
		requirementBytes, err := requireChatEventInteger(fields, "requirement_bytes", false)
		return chatProgressVerification, fmt.Sprintf(
			"Verifying %s against its %d-byte exact requirement", task, requirementBytes,
		), err
	case "coding_task_verified":
		fields, err := exactChatEventFields(event.Message, "task")
		if err != nil {
			return "", "", err
		}
		task, err := requireChatEventToken(fields, "task", 256)
		return chatProgressVerification, "Verified " + task, err
	case "coding_file_started":
		fields, err := exactChatEventFields(event.Message, "path", "stage")
		if err != nil {
			return "", "", err
		}
		path, err := requireChatEventText(fields, "path", 512)
		if err != nil {
			return "", "", err
		}
		stage, err := requireChatEventToken(fields, "stage", 32)
		if stage == "repair" {
			return chatProgressFile, "Repairing " + path, err
		}
		return chatProgressFile, "Constructing " + path, err
	case "coding_file_written":
		fields, err := exactChatEventFields(event.Message, "path", "bytes", "operation", "result")
		if err != nil {
			return "", "", err
		}
		path, err := requireChatEventText(fields, "path", 512)
		if err != nil {
			return "", "", err
		}
		bytes, err := requireChatEventInteger(fields, "bytes", false)
		if err != nil {
			return "", "", err
		}
		operation, err := requireChatEventToken(fields, "operation", 32)
		if err != nil || (operation != "create" && operation != "replace") {
			return "", "", fmt.Errorf("coding file operation %q is not registered", operation)
		}
		if _, err := requireChatEventText(fields, "result", 512); err != nil {
			return "", "", err
		}
		return chatProgressFile, fmt.Sprintf("Accepted %s (%d bytes)", path, bytes), nil
	case "coding_file_deleted":
		fields, err := exactChatEventFields(event.Message, "path", "result")
		if err != nil {
			return "", "", err
		}
		path, err := requireChatEventText(fields, "path", 512)
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "result", 512); err != nil {
			return "", "", err
		}
		return chatProgressFile, "Deleted " + path, nil
	case "coding_file_unchanged":
		fields, err := exactChatEventFields(event.Message, "path")
		path, fieldErr := requireChatEventText(fields, "path", 512)
		return chatProgressFile, "No change required for " + path, firstChatProgressError(err, fieldErr)
	case "coding_file_delete_skipped":
		fields, err := exactChatEventFields(event.Message, "path", "reason")
		if err != nil {
			return "", "", err
		}
		path, err := requireChatEventText(fields, "path", 512)
		if err != nil || fields["reason"] != "missing" {
			return "", "", firstChatProgressError(err, fmt.Errorf("delete skip reason must be missing"))
		}
		return chatProgressFile, "Delete skipped because " + path + " is absent", nil
	case "coding_verification_started":
		fields, err := exactChatEventFields(event.Message, "commands")
		commands, fieldErr := requireChatEventInteger(fields, "commands", false)
		return chatProgressVerification, fmt.Sprintf("Running %d code-selected verification commands", commands), firstChatProgressError(err, fieldErr)
	case "coding_verification_command_passed":
		fields, err := exactChatEventFields(event.Message, "command")
		command, fieldErr := requireChatEventText(fields, "command", 512)
		return chatProgressVerification, "Verification passed: " + command, firstChatProgressError(err, fieldErr)
	case "coding_verification_failed":
		fields, err := exactChatEventFields(event.Message, "command", "diagnostic")
		if err != nil {
			return "", "", err
		}
		command, err := requireChatEventText(fields, "command", 512)
		if err != nil {
			return "", "", err
		}
		diagnostic, err := requireChatEventText(fields, "diagnostic", maxChatProgressRawBytes)
		return chatProgressDiagnostic, "Verification failed for " + command + ": " + boundedChatProgressText(diagnostic), err
	case "coding_static_validation_failed":
		fields, err := exactChatEventFields(event.Message, "diagnostic")
		diagnostic, fieldErr := requireChatEventText(fields, "diagnostic", maxChatProgressRawBytes)
		return chatProgressDiagnostic, "Static validation failed: " + boundedChatProgressText(diagnostic), firstChatProgressError(err, fieldErr)
	case "coding_stage_started", "coding_stage_passed":
		return summarizeChatCodingStage(event)
	case "coding_fragment_repair_guidance_started":
		return summarizeChatCodingRepairGuidance(event.Message)
	case "coding_fragment_correction_started":
		return summarizeChatCodingCorrection(event.Message)
	case "coding_skill_bound":
		return summarizeChatSkillBinding(event.Message)
	case "repository_index_started", "repository_index_ready", "repository_index_failed":
		return summarizeChatRepositoryIndex(event)
	case "repository_change_staged", "repository_change_completed",
		"repository_desired_state_staged", "repository_desired_state_verified":
		return summarizeChatRepositoryChange(event)
	case "repository_verification_command_passed", "repository_verification_baseline_accepted", "repository_verification_plan_accepted":
		return summarizeChatRepositoryVerification(event)
	case "repository_mutation_recovery_started", "repository_mutation_recovered":
		return summarizeChatRepositoryRecovery(event)
	case "coding_repair_selected":
		fields, err := exactChatEventFields(event.Message, "repair", "path", "command")
		if err != nil {
			return "", "", err
		}
		repair, err := requireChatEventInteger(fields, "repair", false)
		if err != nil {
			return "", "", err
		}
		path, err := requireChatEventText(fields, "path", 512)
		return chatProgressReview, fmt.Sprintf("Selected %s for exact repair %d", path, repair), err
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
