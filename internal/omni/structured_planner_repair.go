package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func requestStructuredCommandPayload(ctx context.Context, client CommandDecisionClient, req OllamaChatRequest, step int, onEvent func(StructuredCommandEvent)) (OllamaChatResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= defaultStructuredLLMRequestAttempts; attempt++ {
		resp, err := client.ChatRaw(ctx, req)
		if err == nil {
			if attempt > 1 {
				emitStructuredCommandEvent(onEvent, "structured_llm_request_recovered", "Structured LLM request recovered after retry", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
				})
			}
			return resp, nil
		}
		lastErr = err
		emitStructuredCommandEvent(onEvent, "structured_llm_request_failed", "Structured LLM request failed", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt),
			"error":   truncateStructuredTimelineValue(err.Error()),
		})
		if !isTransientStructuredLLMError(err) || attempt == defaultStructuredLLMRequestAttempts {
			return OllamaChatResponse{}, err
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_backend_unstable", "Ollama backend appears unstable; retrying request", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"attempt":    fmt.Sprintf("%d", attempt),
			"backoff":    structuredLLMRetryBackoff(attempt).String(),
			"diagnosis":  classifyStructuredLLMFailure(err),
			"mitigation": "check journalctl -u ollama; prefer cpu_avx2 or reduce Ollama context/keep_alive if ROCm is crashing",
		})
		select {
		case <-ctx.Done():
			return OllamaChatResponse{}, ctx.Err()
		case <-time.After(structuredLLMRetryBackoff(attempt)):
		}
	}
	return OllamaChatResponse{}, lastErr
}

func structuredLLMRetryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second * time.Duration(1<<min(attempt, 5))
	if delay > maxStructuredLLMBackoff {
		return maxStructuredLLMBackoff
	}
	return delay
}

func repairStructuredPayloadBeforeRouting(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, resp OllamaChatResponse, payload StructuredCommandPayload, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string, survey WorksiteSurvey, onEvent func(StructuredCommandEvent)) (bool, OllamaChatResponse, StructuredCommandPayload, []StructuredObjective, error) {
	if client == nil || !structuredPayloadNeedsImmediateRepair(payload, observations) {
		return false, resp, payload, ledger, nil
	}
	initialValidationErr := immediateStructuredPayloadValidationError(payload, observations, workingDirectory, ledger, survey)
	if initialValidationErr == nil || !isImmediatePlannerRepairValidation(initialValidationErr, ledger) {
		return false, resp, payload, ledger, nil
	}
	currentResp := resp
	currentPayload := payload
	currentLedger := ledger
	for attempt := 1; attempt <= defaultStructuredPlannerRepairAttempts; attempt++ {
		validationErr := immediateStructuredPayloadValidationError(currentPayload, observations, workingDirectory, currentLedger, survey)
		if validationErr == nil {
			return attempt > 1, currentResp, currentPayload, currentLedger, nil
		}
		if !isImmediatePlannerRepairValidation(validationErr, currentLedger) {
			return true, currentResp, currentPayload, currentLedger, nil
		}
		emitStructuredCommandEvent(onEvent, "structured_planner_repair_started", "Planner received immediate validation feedback for isolated repair", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt),
			"command": truncateStructuredTimelineValue(currentPayload.Command),
			"reason":  truncateStructuredTimelineValue(validationErr.Error()),
		})
		repairReq := buildStructuredPlannerRepairRequest(baseReq, prompt, currentResp.Content, currentPayload.Command, validationErr.Error(), currentLedger, observations, workingDirectory)
		nextResp, err := requestStructuredCommandPayload(ctx, client, repairReq, step, onEvent)
		if err != nil {
			return true, currentResp, currentPayload, currentLedger, err
		}
		nextPayload, err := ParseStructuredCommandPayload(nextResp.Content)
		if err != nil {
			return true, currentResp, currentPayload, currentLedger, err
		}
		nextPayload.Command = normalizeStructuredCommand(nextPayload.Command)
		nextLedger := mergePlannerObjectiveLedger(step, currentLedger, nextPayload.ObjectiveLedger, observations, workingDirectory, onEvent)
		emitStructuredCommandEvent(onEvent, "structured_planner_repair_payload_received", "Planner returned repaired structured payload", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"attempt":            fmt.Sprintf("%d", attempt),
			"done":               fmt.Sprintf("%t", nextPayload.Done),
			"ask":                fmt.Sprintf("%t", nextPayload.Ask),
			"tool":               truncateStructuredTimelineValue(nextPayload.Tool),
			"command":            truncateStructuredTimelineValue(nextPayload.Command),
			"pending_objectives": pendingStructuredObjectiveIDs(nextLedger),
		})
		currentResp = nextResp
		currentPayload = nextPayload
		currentLedger = nextLedger
		if immediateStructuredPayloadValidationError(currentPayload, observations, workingDirectory, currentLedger, survey) == nil {
			emitStructuredCommandEvent(onEvent, "structured_planner_repair_accepted", "Planner repaired payload accepted by deterministic pre-validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
			})
			return true, currentResp, currentPayload, currentLedger, nil
		}
	}
	return true, currentResp, currentPayload, currentLedger, nil
}

func structuredPayloadNeedsImmediateRepair(payload StructuredCommandPayload, observations []StructuredCommandObservation) bool {
	if isPatchToolDelegation(payload) || isShellToolDelegation(payload) || payload.Done {
		return false
	}
	if payload.Ask && strings.TrimSpace(payload.Command) == "" {
		return false
	}
	if strings.TrimSpace(payload.Command) == "" {
		return false
	}
	return true
}

func immediateStructuredPayloadValidationError(payload StructuredCommandPayload, observations []StructuredCommandObservation, workingDirectory string, ledger []StructuredObjective, survey WorksiteSurvey) error {
	if !structuredPayloadNeedsImmediateRepair(payload, observations) {
		return nil
	}
	return validateStructuredCommandForRunWithSurvey(payload.Command, observations, workingDirectory, ledger, survey)
}

func validateStructuredCommandForRunWithArchitect(command, prompt, toolTask, patch string, observations []StructuredCommandObservation, workingDirectory string, objectiveLedger []StructuredObjective, survey WorksiteSurvey) error {
	if err := validateStructuredCommandForRunWithSurvey(command, observations, workingDirectory, objectiveLedger, survey); err != nil {
		return err
	}
	if err := validateViteCommandUsesNpmScript(command, workingDirectory, survey); err != nil {
		return err
	}
	architectTargetedTask := strings.Contains(strings.ToLower(toolTask), "implementation architect target root:")
	if !architectTargetedTask && !hasImplementationArchitectProgress(observations) {
		return nil
	}
	signal := strings.TrimSpace(toolTask + "\n" + command + "\n" + patch)
	contract := buildImplementationArchitectContract(prompt, signal, workingDirectory, survey, observations)
	if err := validateCommandAgainstImplementationArchitectContract(command, contract); err != nil {
		return err
	}
	return nil
}

func hasImplementationArchitectProgress(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(obs.Command)), "architect.apply ") {
			return true
		}
		if viteMissingEntryPath(obs.Stderr) != "" || viteMissingEntryPath(obs.Stdout) != "" {
			return true
		}
	}
	return false
}

func validateViteCommandUsesNpmScript(command, workingDirectory string, survey WorksiteSurvey) error {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return nil
	}
	if fields[0] != "vite" {
		return nil
	}
	if survey.PackageManager != packageManagerNPM && survey.PackageManager != packageManagerUnknown && survey.PackageManager != "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(structuredPromptWorkingDirectory(workingDirectory), "package.json")); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(structuredPromptWorkingDirectory(workingDirectory), "node_modules", ".bin", "vite")); err == nil {
		return nil
	}
	return fmt.Errorf("bare vite command rejected: for npm/Vite projects prefer npm scripts such as npm run build, or intentionally use npx vite after verifying node_modules/.bin/vite exists")
}

func isImmediatePlannerRepairValidation(err error, ledger []StructuredObjective) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	if strings.Contains(text, "placeholder-only command does not satisfy app objectives") {
		return true
	}
	return strings.Contains(text, "pure echo command is not command evidence") && pendingObjectivesNeedSubstantiveAppFiles(ledger)
}

func buildStructuredPlannerRepairRequest(baseReq OllamaChatRequest, prompt, rejectedPayload, rejectedCommand, reason string, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string) OllamaChatRequest {
	req := baseReq
	req.Messages = append([]OllamaMessage(nil), baseReq.Messages...)
	payload := struct {
		CurrentPrompt           string                         `json:"current_prompt"`
		RejectedPayload         string                         `json:"rejected_payload"`
		RejectedCommand         string                         `json:"rejected_command"`
		ValidationFeedback      string                         `json:"validation_feedback"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		PendingObjectiveIDs     []string                       `json:"pending_objective_ids,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations,omitempty"`
		RepairRules             []string                       `json:"repair_rules"`
	}{
		CurrentPrompt:           prompt,
		RejectedPayload:         truncateStructuredObservation(rejectedPayload),
		RejectedCommand:         truncateStructuredObservation(rejectedCommand),
		ValidationFeedback:      reason,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(workingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		PendingObjectiveIDs:     structuredObjectiveIDs(pendingStructuredObjectives(ledger)),
		Observations:            compactStructuredObservationsForContext(observations, 6, 600),
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"Repair the rejected payload directly; do not ask another specialist and do not restate the feedback.",
			"The validator feedback is authoritative.",
			"Choose a command, tool, or patch that satisfies pending objectives and avoids the rejected pattern.",
			"If feedback says placeholder-only, write substantive source/build/test file content now.",
			"If feedback says pure echo command, do not print a plan; write or verify substantive files for the pending objectives.",
			"If feedback says dependency scope drift, write requested source files or use existing dependencies instead of installing optional packages.",
			"If feedback says repeated command, inspect completed_actions/observations and choose the next required action.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"validation_feedback":"repair rejected structured payload"}`)
	}
	req.Messages = append(req.Messages,
		OllamaMessage{Role: "assistant", Content: strings.TrimSpace(rejectedPayload)},
		OllamaMessage{Role: "user", Content: string(blob)},
	)
	return req
}
