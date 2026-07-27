package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeStructuredEvaluatorThreshold(value int) int {
	if value <= 0 {
		return defaultEvaluatorThreshold
	}
	if value > 100 {
		return 100
	}
	return value
}

func structuredEvaluationRetryMessage(evaluation StructuredLLMEvaluation, threshold int) string {
	feedback := strings.TrimSpace(evaluation.Feedback)
	if feedback == "" {
		feedback = "planner response was not sufficiently aligned with the active task"
	}
	verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
	return fmt.Sprintf("self-evaluation rejected response: verdict=%s confidence=%d threshold=%d; feedback=%s; try again using the active prompt, planner job, observations, worksite survey, and capability memory", verdict, evaluation.Confidence, threshold, feedback)
}

func structuredEvaluatorValidationScope(ledger []StructuredObjective, observations []StructuredCommandObservation) string {
	if hasSuccessfulStructuredMutation(observations) {
		return "alignment_after_evidence"
	}
	if len(pendingStructuredObjectives(ledger)) > 0 {
		return "current_objective_and_payload_shape"
	}
	return "planner_payload_shape_only"
}

func shouldRunBroadEvaluatorForPlannerPayload(payload StructuredCommandPayload) bool {
	return payload.Done
}

func shouldDeferBroadEvaluatorForArchitectCompletion(payload StructuredCommandPayload, prompt, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) bool {
	if !payload.Done || !hasImplementationArchitectProgress(observations) {
		return false
	}
	signal := strings.TrimSpace(payload.ToolTask + "\n" + payload.Command + "\n" + payload.Patch)
	contract := buildImplementationArchitectContract(prompt, signal, workingDir, survey, observations)
	return hasImplementationArchitectContract(contract) && contract.CurrentItem != nil
}

func runFinalBroadEvaluatorAfterTypedCompletion(ctx context.Context, step int, prompt string, evaluator StructuredLLMResponseEvaluator, threshold int, cfg structuredCommandDecisionRunConfig, survey WorksiteSurvey, ledger []StructuredObjective, observations []StructuredCommandObservation, answer string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if evaluator == nil || result == nil || len(result.WorkItems) == 0 {
		return true, nil
	}
	evaluation, err := evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
		Step:             step,
		UserPrompt:       prompt,
		ValidationScope:  "alignment_after_typed_recursive_completion",
		LLMResponse:      answer,
		Observations:     observations,
		CompletedActions: completedActionsFromState(ledger, observations),
		LoopState:        structuredLoopStateFromState(ledger, observations),
		SessionMemories:  cfg.SessionMemories,
		WorksiteSurvey:   survey,
	})
	if err != nil {
		wrapped := fmt.Errorf("structured response evaluator failed after typed completion: %w", err)
		emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Final broad evaluator failed after typed completion; structured execution stopped", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return false, wrapped
	}
	if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
		wrapped := fmt.Errorf("structured response evaluator returned inconsistent scoring after typed completion: %w", consistencyErr)
		emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Final broad evaluator returned inconsistent scoring; structured execution stopped", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
			"error":      truncateStructuredTimelineValue(consistencyErr.Error()),
		})
		return false, wrapped
	}
	emitStructuredCommandEvent(onEvent, "structured_response_evaluated", "Structured response evaluator scored final typed completion", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"verdict":    evaluation.Verdict,
		"confidence": fmt.Sprintf("%d", evaluation.Confidence),
		"threshold":  fmt.Sprintf("%d", threshold),
		"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
	})
	if strings.EqualFold(strings.TrimSpace(evaluation.Verdict), "revise") || evaluation.Confidence < threshold {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:                 step,
			ExitCode:             1,
			Stderr:               "final broad evaluator rejected typed completion: " + strings.TrimSpace(evaluation.Feedback),
			EvaluationConfidence: evaluation.Confidence,
			EvaluationFeedback:   evaluation.Feedback,
		})
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
		result.Answer = ""
		emitStructuredCommandEvent(onEvent, "structured_response_rejected", "Final broad evaluator rejected typed completion", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"verdict":    evaluation.Verdict,
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"threshold":  fmt.Sprintf("%d", threshold),
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
		})
		return false, nil
	}
	return true, nil
}

func shouldBypassEvaluatorForArchitectImplementation(rawResponse, prompt, workingDir string, survey WorksiteSurvey, observations []StructuredCommandObservation) bool {
	payload, err := ParseStructuredCommandPayload(rawResponse)
	if err != nil {
		return false
	}
	if payload.Done || payload.Ask {
		return false
	}
	if structuredCommandLooksReadOnlyEvidence(payload.Command) {
		return false
	}
	if !strings.Contains(strings.ToLower(payload.ToolTask), "implementation architect target root:") && !hasImplementationArchitectProgress(observations) {
		return false
	}
	signal := strings.TrimSpace(payload.ToolTask + "\n" + payload.Command + "\n" + payload.Patch)
	if signal == "" {
		return false
	}
	contract := buildImplementationArchitectContract(prompt, signal, workingDir, survey, observations)
	return hasImplementationArchitectContract(contract) && contract.CurrentItem != nil
}

func evaluateAndRepairPlannerResponse(ctx context.Context, step int, prompt string, client CommandDecisionClient, baseReq OllamaChatRequest, resp OllamaChatResponse, evaluator StructuredLLMResponseEvaluator, evaluatorThreshold int, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, ledger []StructuredObjective, observations []StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, OllamaChatResponse, bool, error) {
	if evaluator == nil {
		return true, resp, false, nil
	}
	currentResp := resp
	evaluationAttempts := 0
	repairAttempts := 0
	for {
		if len(allPrepBriefs(cfg.PrepContext)) > 0 || len(cfg.PrepContext.Evidence) > 0 {
			emitStructuredCommandEvent(onEvent, "prep_context_attached_to_specialist", "Preparation context attached to evaluator", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"role":     "evaluator",
				"briefs":   fmt.Sprintf("%d", len(allPrepBriefs(cfg.PrepContext))),
				"evidence": fmt.Sprintf("%d", len(cfg.PrepContext.Evidence)),
			})
		}
		evaluationAttempts++
		evaluation, evalErr := evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
			Step:             step,
			UserPrompt:       prompt,
			PlannerJob:       structuredCommandPlannerJobSummary(),
			ValidationScope:  structuredEvaluatorValidationScope(ledger, result.Observations),
			LLMResponse:      currentResp.Content,
			Observations:     result.Observations,
			CompletedActions: completedActionsFromState(ledger, result.Observations),
			LoopState:        structuredLoopStateFromState(ledger, result.Observations),
			SessionMemories:  cfg.SessionMemories,
			WorksiteSurvey:   worksiteSurvey,
		})
		if evalErr != nil {
			wrapped := fmt.Errorf("structured response evaluator failed: %w", evalErr)
			emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator failed; structured execution stopped", map[string]string{
				"step":  fmt.Sprintf("%d", step),
				"error": truncateStructuredTimelineValue(evalErr.Error()),
			})
			return true, currentResp, false, wrapped
		}
		if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
			wrapped := fmt.Errorf("structured response evaluator returned inconsistent scoring: %w", consistencyErr)
			emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator returned inconsistent scoring; structured execution stopped", map[string]string{
				"step":       fmt.Sprintf("%d", step),
				"confidence": fmt.Sprintf("%d", evaluation.Confidence),
				"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
				"error":      truncateStructuredTimelineValue(consistencyErr.Error()),
			})
			return true, currentResp, false, wrapped
		}
		if normalizeStructuredEvaluationVerdict(evaluation.Verdict) == "accept" && structuredEvaluationFeedbackSuggestsHardReject(evaluation.Feedback+" "+evaluation.BlockingReason) {
			evaluation.Verdict = "reject"
		}
		verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
		emitStructuredCommandEvent(onEvent, "structured_response_evaluated", "Structured response evaluator scored planner output", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
			"verdict":    verdict,
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
		})
		if verdict != "reject" && verdict != "revise" && evaluation.Confidence >= evaluatorThreshold {
			if repairAttempts > 0 {
				emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_accepted", "Planner repair accepted by evaluator", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", repairAttempts),
				})
			}
			return true, currentResp, false, nil
		}
		if verdict == "revise" && repeatedStructuredEvaluationFeedback(evaluation, result.Observations) {
			emitStructuredCommandEvent(onEvent, "structured_evaluator_loop_bypassed", "Repeated evaluator revise feedback bypassed for deterministic validation", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"feedback": truncateStructuredTimelineValue(evaluation.Feedback),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:                 step,
				RejectedResponse:     truncateStructuredObservation(currentResp.Content),
				EvaluationConfidence: evaluation.Confidence,
				EvaluationFeedback:   truncateStructuredObservation(evaluation.Feedback),
				ExitCode:             1,
				Stderr:               "anti_loop: evaluator repeated the same revise feedback; evaluator bypassed for this planner output. Continue with deterministic command validation, objective ledger, worksite survey, and observed command evidence.",
			})
			return true, currentResp, true, nil
		}
		appendEvaluatorRejectionObservation(step, currentResp.Content, evaluation, evaluatorThreshold, result)
		emitStructuredCommandEvent(onEvent, "structured_response_rejected", "Structured response rejected by evaluator", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"confidence": fmt.Sprintf("%d", evaluation.Confidence),
			"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
			"verdict":    verdict,
			"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
		})
		if repairAttempts >= defaultEvaluatorPlannerRepairAttempts || client == nil {
			return false, currentResp, false, nil
		}
		repaired := false
		for repairAttempts < defaultEvaluatorPlannerRepairAttempts {
			repairAttempts++
			emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_started", "Planner received evaluator feedback for local repair", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"attempt":  fmt.Sprintf("%d", repairAttempts),
				"verdict":  verdict,
				"feedback": truncateStructuredTimelineValue(evaluation.Feedback),
			})
			repairReq := buildStructuredPlannerEvaluatorRepairRequest(baseReq, prompt, currentResp.Content, evaluation, evaluatorThreshold, ledger, result.Observations, cfg.CurrentWorkingDirectory)
			nextResp, err := requestStructuredCommandPayload(ctx, client, repairReq, step, onEvent)
			if err != nil {
				return false, currentResp, false, err
			}
			emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_payload_received", "Planner returned evaluator-repaired payload", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", repairAttempts),
				"payload": truncateStructuredTimelineValue(nextResp.Content),
			})
			if constraintErr := validateStructuredEvaluatorRepairPayload(nextResp.Content, evaluation, result.Observations, cfg.CurrentWorkingDirectory, ledger, worksiteSurvey); constraintErr != nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedResponse: truncateStructuredObservation(nextResp.Content),
					CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(nextResp.Content, constraintErr.Error()),
					ExitCode:         1,
					Stderr:           "evaluator repair rejected: " + constraintErr.Error(),
				})
				emitStructuredCommandEvent(onEvent, "structured_evaluator_repair_rejected", "Planner repair ignored evaluator constraints", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", repairAttempts),
					"reason":  truncateStructuredTimelineValue(constraintErr.Error()),
				})
				continue
			}
			currentResp = nextResp
			repaired = true
			break
		}
		if !repaired {
			return false, currentResp, false, nil
		}
		if evaluationAttempts > defaultEvaluatorPlannerRepairAttempts+1 {
			return false, currentResp, false, nil
		}
	}
}

func appendEvaluatorRejectionObservation(step int, rawResponse string, evaluation StructuredLLMEvaluation, threshold int, result *CommandDecisionResult) {
	verdict := normalizeStructuredEvaluationVerdict(evaluation.Verdict)
	reason := structuredEvaluationRetryMessage(evaluation, threshold)
	if verdict == "reject" {
		reason = "scope_drift: evaluator rejected planner output; " + reason
	}
	rejectedCommand := ""
	if rejectedPayload, parseErr := ParseStructuredCommandPayload(rawResponse); parseErr == nil {
		rejectedCommand = truncateStructuredObservation(rejectedPayload.Command)
	}
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:                 step,
		RejectedResponse:     truncateStructuredObservation(rawResponse),
		RejectedCommand:      rejectedCommand,
		EvaluationConfidence: evaluation.Confidence,
		EvaluationFeedback:   truncateStructuredObservation(evaluation.Feedback),
		CapabilityMemory:     structuredCapabilityMemoryForRejectedResponse(rawResponse, evaluation.Feedback),
		ExitCode:             1,
		Stderr:               reason,
	})
}

func buildStructuredPlannerEvaluatorRepairRequest(baseReq OllamaChatRequest, prompt, rejectedPayload string, evaluation StructuredLLMEvaluation, threshold int, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDirectory string) OllamaChatRequest {
	req := baseReq
	req.Messages = append([]OllamaMessage(nil), baseReq.Messages...)
	hardConstraints := structuredEvaluatorRepairHardConstraints(evaluation)
	payload := struct {
		CurrentPrompt           string                         `json:"current_prompt"`
		RejectedPayload         string                         `json:"rejected_payload"`
		EvaluatorVerdict        string                         `json:"evaluator_verdict"`
		EvaluatorConfidence     int                            `json:"evaluator_confidence"`
		EvaluatorThreshold      int                            `json:"evaluator_threshold"`
		EvaluatorFeedback       string                         `json:"evaluator_feedback"`
		BlockingReason          string                         `json:"blocking_reason,omitempty"`
		CurrentWorkingDirectory string                         `json:"current_working_directory"`
		ObjectiveLedger         []StructuredObjective          `json:"objective_ledger,omitempty"`
		PendingObjectiveIDs     []string                       `json:"pending_objective_ids,omitempty"`
		Observations            []StructuredCommandObservation `json:"observations,omitempty"`
		HardConstraints         []string                       `json:"hard_constraints,omitempty"`
		RepairRules             []string                       `json:"repair_rules"`
	}{
		CurrentPrompt:           prompt,
		RejectedPayload:         truncateStructuredObservation(rejectedPayload),
		EvaluatorVerdict:        normalizeStructuredEvaluationVerdict(evaluation.Verdict),
		EvaluatorConfidence:     evaluation.Confidence,
		EvaluatorThreshold:      threshold,
		EvaluatorFeedback:       evaluation.Feedback,
		BlockingReason:          evaluation.BlockingReason,
		CurrentWorkingDirectory: structuredPromptWorkingDirectory(workingDirectory),
		ObjectiveLedger:         mergeStructuredObjectiveLedger(nil, ledger),
		PendingObjectiveIDs:     structuredObjectiveIDs(pendingStructuredObjectives(ledger)),
		Observations:            compactStructuredObservationsForContext(observations, 6, 600),
		HardConstraints:         hardConstraints,
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"The evaluator feedback is authoritative for this repair attempt.",
			"Repair the rejected planner payload directly; do not restate or argue with the feedback.",
			"Choose the next concrete command, tool delegation, or patch that aligns with the active prompt and observed evidence.",
			"Hard constraints are deterministic validation rules; violating them rejects the repair without another evaluator pass.",
			"Do not return the same rejected response.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"evaluator_feedback":"repair rejected planner payload"}`)
	}
	req.Messages = append(req.Messages,
		OllamaMessage{Role: "assistant", Content: strings.TrimSpace(rejectedPayload)},
		OllamaMessage{Role: "user", Content: string(blob)},
	)
	return req
}

func structuredEvaluatorRepairHardConstraints(evaluation StructuredLLMEvaluation) []string {
	text := strings.ToLower(evaluation.Feedback + " " + evaluation.BlockingReason)
	constraints := []string{}
	if strings.Contains(text, "patch.apply") || strings.Contains(text, "patch apply") {
		constraints = append(constraints, `Return tool="patch.apply", command="", and a non-empty unified diff in patch.`)
	}
	if strings.Contains(text, "create or update a file") ||
		strings.Contains(text, "source edit") ||
		strings.Contains(text, "source edits") ||
		strings.Contains(text, "substantive source") ||
		strings.Contains(text, "write substantive") {
		constraints = append(constraints, "The payload must create or modify substantive source/build/test file content, not print a plan, install optional dependencies, or create placeholder-only files.")
	}
	return constraints
}

func validateStructuredEvaluatorRepairPayload(raw string, evaluation StructuredLLMEvaluation, observations []StructuredCommandObservation, workingDirectory string, ledger []StructuredObjective, survey WorksiteSurvey) error {
	payload, err := ParseStructuredCommandPayload(raw)
	if err != nil {
		return fmt.Errorf("repair payload is not valid structured command JSON: %w", err)
	}
	text := strings.ToLower(evaluation.Feedback + " " + evaluation.BlockingReason)
	if strings.Contains(text, "patch.apply") || strings.Contains(text, "patch apply") {
		if !isPatchToolDelegation(payload) {
			return fmt.Errorf("evaluator required patch.apply; repair must return tool=patch.apply with command empty and a non-empty unified diff patch")
		}
		return nil
	}
	if strings.Contains(text, "create or update a file") ||
		strings.Contains(text, "source edit") ||
		strings.Contains(text, "source edits") ||
		strings.Contains(text, "substantive source") ||
		strings.Contains(text, "write substantive") {
		if payload.Done || payload.Ask {
			return fmt.Errorf("evaluator required substantive file work; repair cannot ask or request completion")
		}
		if isShellToolDelegation(payload) || isPatchToolDelegation(payload) {
			return nil
		}
		if err := validateStructuredCommandForRunWithSurvey(payload.Command, observations, workingDirectory, ledger, survey); err != nil {
			return err
		}
	}
	return nil
}

func structuredCommandPlannerJobSummary() string {
	return strings.Join([]string{
		"Return strict JSON for the next command-planning step.",
		"Use schema {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}.",
		"Use {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"} only after successful command evidence.",
		"Commands must gather evidence, inspect state, create requested output, or verify results.",
		"Do not simulate final answers with echo/printf apologies or claims that real-time information is unavailable.",
		"Use shell commands and public unauthenticated sources for current facts when needed.",
	}, " ")
}

func buildStructuredLLMEvaluationRequest(input StructuredLLMEvaluationInput) OllamaChatRequest {
	payload := struct {
		Step             int                            `json:"step"`
		Job              string                         `json:"planner_job"`
		ValidationScope  string                         `json:"validation_scope"`
		UserPrompt       string                         `json:"user_prompt"`
		LLMResponse      string                         `json:"llm_response"`
		Observations     []StructuredCommandObservation `json:"observations"`
		CompletedActions []CompletedAction              `json:"completed_actions,omitempty"`
		LoopState        StructuredLoopState            `json:"loop_state,omitempty"`
		SessionMemories  []SessionMemory                `json:"session_memories,omitempty"`
		WorksiteSurvey   WorksiteSurvey                 `json:"worksite_survey"`
	}{
		Step:             input.Step,
		Job:              input.PlannerJob,
		ValidationScope:  input.ValidationScope,
		UserPrompt:       input.UserPrompt,
		LLMResponse:      input.LLMResponse,
		Observations:     compactStructuredObservationsForContext(input.Observations, 8, 650),
		CompletedActions: input.CompletedActions,
		LoopState:        input.LoopState,
		SessionMemories:  compactSessionMemoriesForStructuredContext(input.SessionMemories, 6, 600),
		WorksiteSurvey:   input.WorksiteSurvey,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a tiny strict evaluator.",
					"Return JSON only with schema {\"verdict\":\"accept|revise|reject\",\"confidence\":0,\"blocking_reason\":\"\",\"feedback\":\"\"}.",
					"confidence must be an integer from 0 to 100.",
					"Score whether llm_response is on track for planner_job and user_prompt.",
					"Validation scope is authoritative: planner_payload_shape_only checks whether the next payload is executable and in scope; current_objective_and_payload_shape checks only the current objective and payload; alignment_after_evidence checks final fit after implementation evidence exists.",
					"Do not act as a broad product critic during planner_payload_shape_only or current_objective_and_payload_shape; do not add new feature expectations beyond user_explicit, recipe_required, detected_project, or evidence_required_prerequisite objectives.",
					"Treat completed_actions as authoritative progress; reject planner output that repeats completed work instead of advancing pending objectives.",
					"Treat loop_state as the loop monitor output; reject or revise responses that keep repeating its blocked action pattern.",
					"Do not treat loop_state.repeated_command as a ban; it is evidence that prior validation disliked a proposal or that a failed command needs correction.",
					"Use verdict=reject for semantic mismatch, scope drift, or contradictions with WorksiteSurvey.",
					"Use verdict=revise when the response may be salvageable but must not execute yet.",
					"Scoring rubric: 90-100 clearly on track or complete, 70-89 mostly on track, 40-69 uncertain or incomplete, 0-39 off track.",
					"If feedback says on track, successfully completed, or correctly answered, confidence must be at least 80.",
					"If confidence is below 70, feedback must state what is missing or wrong and must not say the response is on track.",
					"Do not solve the user's task.",
					"Do not penalize a proposed command merely because it has not executed yet; the runtime executes accepted commands.",
					"For empty-workspace app build tasks with documentation_brief prep, revise any response that only checks compiler availability, fetches documentation, or states that the workspace is empty; the retry should write source/build/test files from prep evidence.",
					"For proof-first tasks, prefer revise when the planner jumps straight to implementation without creating or updating a focused test, smoke test, or deterministic verification probe, but accept a command that creates the proof and minimal implementation together.",
					"For proof-first tasks, revise proof plans that expand beyond user_explicit, recipe_required, or evidence_required_prerequisite objectives.",
					"Revise attempts to weaken, delete, skip, or rewrite a validated test/probe unless the payload gives validator-approved syntax/tooling correction, user-request change, or equivalent framework migration evidence.",
					"Prefer the lightest proof type that gives a clear signal: unit/integration test, smoke test, golden output, compiler check, lint check, source verification, or manual evaluator acceptance.",
					"Revise commands that only print status text such as echo/printf when pending objectives require implementation; they are not command evidence.",
					"Revise placeholder-only mkdir/touch scaffolds when app, component, CRUD, UI, source, or storage objectives remain; the retry must write substantive source/build/test file content.",
					"Reject unrequested dependency installs when pending objectives now require implementation work; do not add packages just because they are common.",
					"Give low confidence when the response ignores the active prompt, answers from memory, refuses a capability that shell/public sources provide, returns done without evidence, or emits a command that only prints an answer/apology.",
					"Give low confidence when memory or prior preferences expand dependencies, frameworks, files, services, architecture, or deployment targets beyond the current prompt or selected recipe.",
					"Reject when a command creates or scaffolds a new project but WorksiteSurvey says the operation is modify_existing_project or fix_existing_project.",
					"Give low confidence for obviously invalid shell command syntax or repeated commands already shown failing in observations.",
					"feedback must be one concise sentence explaining how the planner should retry.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"verdict":         map[string]interface{}{"type": "string", "enum": []string{"accept", "revise", "reject"}},
				"confidence":      map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 100},
				"blocking_reason": map[string]interface{}{"type": "string"},
				"feedback":        map[string]interface{}{"type": "string"},
			},
			"required": []string{"confidence", "feedback"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 128,
		},
	}
}
