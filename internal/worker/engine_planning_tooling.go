package worker

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
)

func (s *Service) emitStepStream(stepID int64, stream, message string) {
	key := "tool_stdout"
	if strings.EqualFold(strings.TrimSpace(stream), "stderr") {
		key = "tool_stderr"
	}
	s.emitStepContext(stepID, key, strings.TrimSpace(message))
}

func (s *Service) emitStepContext(stepID int64, key, value string) {
	s.emitStepContextWithBudget(stepID, key, value, 1800)
}

func (s *Service) emitStepContextWithBudget(stepID int64, key, value string, maxChars int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if maxChars <= 0 {
		maxChars = 1800
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepEventWriteTimeout)
	defer cancel()
	if err := s.repo.AddStepContext(ctx, stepID, key, trimForBudget(value, maxChars)); err != nil {
		s.logger.Printf("step=%d context key=%s write error: %v", stepID, key, err)
	}
}

func (s *Service) llmGenerateWithTrace(ctx context.Context, stepID int64, scope string, modelName string, prompt string) (string, error) {
	scope = safeLine(strings.TrimSpace(scope), "llm")
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("llm model is required for scope %q", scope)
	}
	prompt = strings.TrimSpace(prompt)
	s.emitStepEvent(stepID, "llm_prompt", fmt.Sprintf("scope=%s model=%s chars=%d", scope, modelName, len(prompt)))
	s.emitStepContextWithBudget(stepID, "llm_prompt", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("prompt_chars=%d", len(prompt)),
		prompt,
	}, "\n"), 14000)

	raw, err := s.llmGenerateSingleAttempt(ctx, stepID, scope, modelName, prompt, 1)
	if err == nil {
		return raw, nil
	}
	attemptErrors := []string{fmt.Sprintf("%s: %s", modelName, trimForBudget(err.Error(), 240))}
	if shouldRetrySameModelAfterCreateEOF(err) {
		s.emitStepEvent(stepID, "llm_retry_same_model", fmt.Sprintf("scope=%s model=%s reason=create_eof", scope, modelName))
		raw, retryErr := s.llmGenerateSingleAttempt(ctx, stepID, scope, modelName, prompt, 2)
		if retryErr == nil {
			return raw, nil
		}
		attemptErrors = append(attemptErrors, fmt.Sprintf("%s(retry): %s", modelName, trimForBudget(retryErr.Error(), 240)))
	}
	finalErr := fmt.Errorf("llm generate failed after %d attempt(s) with configured model %q: %s", len(attemptErrors), modelName, strings.Join(attemptErrors, " | "))
	s.emitStepEvent(stepID, "llm_error", fmt.Sprintf("scope=%s model=%s", scope, modelName))
	s.emitStepContextWithBudget(stepID, "llm_error", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		"error=" + trimForBudget(finalErr.Error(), 1400),
	}, "\n"), 3200)
	return "", finalErr
}

func (s *Service) llmGenerateSingleAttempt(ctx context.Context, stepID int64, scope string, modelName string, prompt string, attempt int) (string, error) {
	started := time.Now()
	stopHeartbeat := s.startProgressHeartbeat(ctx, stepID, fmt.Sprintf("llm:%s:attempt-%d", safeLine(scope, "generation"), attempt))
	defer stopHeartbeat()
	prepared, err := s.llm.PrepareContextModel(ctx, modelName, prompt)
	if err != nil {
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, time.Since(started))
		return "", err
	}
	defer s.llm.CleanupPreparedModel(prepared)
	prepared.PromptHint = resolvePreparedPromptHint(scope, prompt, prepared.PromptHint)
	prepared.MaxOutputTokens = v3OutputTokenBudget(scope)
	prepared.ContextTokens = s.inferenceContextTokens
	prepared.ResponseFormat = responseFormatForScope(scope)
	if err := llm.ValidateInferenceBudget(prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt, prepared.PromptHint); err != nil {
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, time.Since(started))
		return "", fmt.Errorf("prepare inference budget for scope %q: %w", scope, err)
	}
	s.emitStepEvent(stepID, "llm_model_prepared", fmt.Sprintf("scope=%s model=%s context_model=%s", scope, modelName, safeLine(prepared.ContextModel, "unknown")))
	s.emitStepContextWithBudget(stepID, "llm_model_prepare", strings.Join([]string{
		"scope=" + scope,
		"base_model=" + safeLine(prepared.BaseModel, modelName),
		"context_model=" + safeLine(prepared.ContextModel, "unknown"),
		"modelfile_path=" + safeLine(prepared.ModelfilePath, "unknown"),
		"prompt_hint=" + safeLine(trimForBudget(prepared.PromptHint, 420), "system_instructions_only"),
		fmt.Sprintf("max_output_tokens=%d", prepared.MaxOutputTokens),
		fmt.Sprintf("context_tokens=%d", prepared.ContextTokens),
		"response_format=" + safeLine(prepared.ResponseFormat, "text"),
	}, "\n"), 3200)

	raw, err := s.llm.GeneratePrepared(ctx, prepared)
	latency := time.Since(started)
	if err != nil {
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return "", err
	}

	response := strings.TrimSpace(raw)
	s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, true, nil, latency)
	s.emitStepEvent(stepID, "llm_response", fmt.Sprintf("scope=%s model=%s chars=%d", scope, modelName, len(response)))
	s.emitStepContextWithBudget(stepID, "llm_response", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("response_chars=%d", len(response)),
		response,
	}, "\n"), 14000)
	return raw, nil
}

func responseFormatForScope(scope string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(scope)), "v3_") {
		return llm.ResponseFormatJSON
	}
	return ""
}

func v3OutputTokenBudget(scope string) int {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch {
	case strings.HasPrefix(scope, "v3_implementation_manifest"):
		return 2048
	case strings.HasPrefix(scope, "v3_work_item_writer_"):
		return 8192
	case strings.HasPrefix(scope, "v3_work_item_review_"),
		strings.HasPrefix(scope, "v3_failure_triage_"):
		return 1024
	case strings.HasPrefix(scope, "v3_subtask_tool_"):
		return 4096
	case strings.HasPrefix(scope, "v3_intent_parse"),
		strings.HasPrefix(scope, "v3_planning"),
		strings.HasPrefix(scope, "v3_verification"):
		return 2048
	case strings.HasPrefix(scope, "v3_analysis"),
		strings.HasPrefix(scope, "v3_response"):
		return 1024
	case strings.HasPrefix(scope, "v3_"):
		return 768
	default:
		return 0
	}
}

func resolvePreparedPromptHint(scope string, prompt string, preparedHint string) string {
	hint := strings.TrimSpace(preparedHint)
	if hint != "" && !promptHintNeedsScopeOverride(hint) {
		return hint
	}
	if scoped := deriveScopePromptHint(scope, prompt); scoped != "" {
		return scoped
	}
	if hint == "" {
		return "Return only the requested output."
	}
	return hint
}

func promptHintNeedsScopeOverride(hint string) bool {
	hint = strings.ToLower(strings.TrimSpace(hint))
	if hint == "" {
		return true
	}
	if strings.HasPrefix(hint, "user request:") || strings.HasPrefix(hint, "user feedback:") {
		return false
	}
	return true
}

func deriveScopePromptHint(scope string, prompt string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return ""
	}
	if strings.HasPrefix(scope, "v3_") && strings.Contains(prompt, "SPECIALIST_INVOCATION:") {
		if feedback := llm.ExtractPromptBlock(prompt, "DIRECT_FEEDBACK"); feedback != "" && feedback != "(empty)" {
			return llm.TruncatePromptHint(feedback, 1400)
		}
		return "Begin the control-plane-assigned work now."
	}

	goal := extractPromptLabelValue(prompt, "GOAL:", 220)
	instruction := extractPromptLabelValue(prompt, "Instruction:", 220)

	withGoal := func(base string) string {
		base = strings.TrimSpace(base)
		if base == "" {
			return ""
		}
		if goal != "" {
			return trimForBudget(base+" Goal: "+goal, 420)
		}
		return trimForBudget(base, 420)
	}
	withInstruction := func(base string) string {
		base = strings.TrimSpace(base)
		if base == "" {
			return ""
		}
		if instruction != "" {
			return trimForBudget(base+" Instruction: "+instruction, 420)
		}
		return trimForBudget(base, 420)
	}

	switch {
	case strings.HasPrefix(scope, "tournament_leaf_summary_"):
		return withGoal("Assess CHUNK relevance to GOAL and return RELEVANT, CONFIDENCE, SUMMARY, and EVIDENCE.")
	case strings.HasPrefix(scope, "tournament_leaf_verify_"):
		return withGoal("Validate CLAIMED_SUMMARY against ORIGINAL_CHUNK and return SUPPORTED, CORRECTED_SUMMARY, and RATIONALE.")
	case strings.HasPrefix(scope, "tournament_round_"):
		return withGoal("Condense evidence summaries for GOAL with no speculation.")
	case strings.HasPrefix(scope, "verify_evaluate_"):
		return withInstruction("Return strict JSON verification output: status, confidence, summary, gaps, and cannot_complete_reason.")
	case strings.HasPrefix(scope, "verify_revise_"):
		return withInstruction("Revise the assistant response using verification findings. Return revised response text only.")
	case strings.HasPrefix(scope, "memory_inference"):
		return withInstruction("Return strict JSON durable memories with procedural, instruction, and preference arrays.")
	}

	return ""
}

func extractPromptLabelValue(prompt string, label string, maxChars int) string {
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		return ""
	}

	lines := strings.Split(prompt, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.ToLower(strings.TrimSpace(lines[i])) != label {
			continue
		}
		values := make([]string, 0, 4)
		for j := i + 1; j < len(lines); j++ {
			clean := strings.TrimSpace(lines[j])
			if clean == "" {
				if len(values) > 0 {
					break
				}
				continue
			}
			if len(values) > 0 && strings.HasSuffix(clean, ":") {
				break
			}
			values = append(values, clean)
			if len(strings.Join(values, " ")) > maxChars*2 {
				break
			}
		}
		if len(values) == 0 {
			return ""
		}
		return trimForBudget(strings.Join(values, " "), maxChars)
	}
	return ""
}

func (s *Service) runPlanStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	replanFeedback := trimForBudget(metadataString(claim.Job.Metadata, "replan_feedback"), 1200)
	feedback := trimForBudget(strings.TrimSpace(strings.Join([]string{
		replanFeedback,
		contexts["user_feedback"],
	}, "\n")), 1200)
	autonomy := autonomyEnabled(claim.Job)
	persistent := persistentExecutionEnabled(claim.Job)
	forceFreshExternal := shouldForceFreshWebSearch(claim.Job.Instruction, feedback)
	s.emitStepEvent(claim.Step.ID, "plan_begin", fmt.Sprintf("autonomy=%s instruction_chars=%d", resolveAutonomyMode(claim.Job), len(strings.TrimSpace(claim.Job.Instruction))))
	if isLowSignalChatInstruction(claim.Job.Instruction, claim.Job.Pipeline) && strings.TrimSpace(replanFeedback) == "" {
		plan := `{"goal":"Respond to a brief conversational check-in","tasks":["Reply briefly and naturally.","Invite a concrete next request if needed."],"needs_external_info":false,"required_tools":[],"clarifications":[],"done_when":["User receives a concise direct response"]}`
		s.emitStepEvent(claim.Step.ID, "plan_ready", "strategy=low_signal tasks=2")
		return s.repo.CompleteStep(ctx, claim.Step.ID, plan, "plan", plan)
	}
	if !persistent && autonomy && isFollowUpStatusCheckInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		plan := `{"goal":"Answer completion status for the previous turn","tasks":["Inspect parent job metadata/result.","Reply with direct yes/no status.","Avoid speculative replanning."],"needs_external_info":false,"required_tools":[],"clarifications":[],"done_when":["User gets a direct status answer with next command if needed."]}`
		s.emitStepEvent(claim.Step.ID, "plan_ready", "strategy=followup_status tasks=3")
		return s.repo.CompleteStep(ctx, claim.Step.ID, plan, "plan", plan)
	}
	planDefault := s.specialistModel(claim.Job, specialist.RolePlannerSpecialist, s.models.Plan)
	modelName := s.pickThinkingModel(claim.Job, contexts, metadataModel(claim.Job, "model_plan", planDefault))
	s.emitStepEvent(claim.Step.ID, "plan_model", "model="+modelName)

	goal := strings.TrimSpace(claim.Job.Instruction)
	if goal == "" {
		goal = "produce an executable plan"
	}
	preparedContexts, err := s.prepareTournamentContexts(ctx, claim.Step.ID, modelName, goal, []tournamentContextRequest{
		{SourceKey: "recent_conversation", Value: contexts["recent_conversation"], Budget: 1400},
		{SourceKey: "retrieval", Value: contexts["retrieval"], Budget: 1200},
		{SourceKey: "tooling", Value: contexts["tooling"], Budget: 1200},
		{SourceKey: "workspace", Value: contexts["workspace"], Budget: 1600},
	})
	if err != nil {
		return err
	}
	planRecentConversation := preparedContexts["recent_conversation"]
	planRetrievedMemory := preparedContexts["retrieval"]
	planTooling := preparedContexts["tooling"]
	planWorkspace := preparedContexts["workspace"]
	planTags := trimForBudget(contexts["tags"], 400)
	actionCatalog := plannerActionCatalog(claim.Job)
	specialistAssignments := plannerSpecialistAssignments(claim.Job)

	plannerPrompt := strings.Join([]string{
		"You are a task planner for an autonomous execution pipeline.",
		antiRoleplayInstructionForPipeline(claim.Job.Pipeline),
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", claim.Job.Instruction, feedback),
		`Return JSON only with schema: {"goal":"...", "tasks":["..."], "needs_external_info":bool, "required_tools":["..."], "clarifications":["..."], "done_when":["..."]}`,
		"tasks: break the work into the smallest practical executable micro-steps.",
		"tasks: use 8-14 steps for non-trivial requests; use 3-5 only for truly trivial requests.",
		"tasks: each step should be a single concrete action that is easy to verify.",
		"Sequence only executable actions grounded in the available action catalog.",
		"done_when: 2-4 measurable completion conditions.",
		"required_tools: command names if specific tooling is required (npm, go, composer, python, docker, etc).",
		"clarifications: ask only when execution is unsafe/destructive or impossible without an explicit user decision.",
		"needs_external_info should be true only if current memory likely is insufficient and external references/web are needed.",
		"If USER_INSTRUCTION/USER_FEEDBACK explicitly asks for web search or says memory/context is stale/wrong, set needs_external_info=true.",
		"Context precedence:",
		"1) USER_INSTRUCTION and USER_FEEDBACK are authoritative.",
		"2) ACTION_CATALOG defines actions available in this run. Do not invent actions beyond it.",
		"3) TOOLING_CONTEXT and WORKSPACE_CONTEXT are current-run execution research.",
		"4) RETRIEVED_MEMORY_CONTEXT is historical and may be stale or hypothetical.",
		"5) Do not add deployment/testing/tool tasks from memory unless USER_INSTRUCTION explicitly asks for them.",
		"If AUTONOMY is on, prefer sensible defaults and avoid clarification questions unless safety-critical.",
		promptBlock("CURRENT_TIME_CONTEXT", currentTimeContextFromMetadata(claim.Job)),
		promptBlock("AUTONOMY_MODE", resolveAutonomyMode(claim.Job)),
		promptBlock("ACTION_CATALOG", actionCatalog),
		promptBlock("SPECIALIST_ASSIGNMENTS", specialistAssignments),
		promptBlock("USER_INSTRUCTION", claim.Job.Instruction),
		promptBlock("USER_FEEDBACK", feedback),
		promptBlock("RECENT_CONVERSATION", planRecentConversation),
		promptBlock("TAGS", planTags),
		promptBlock("TOOLING_CONTEXT", planTooling),
		promptBlock("WORKSPACE_CONTEXT", planWorkspace),
		promptBlock("RETRIEVED_MEMORY_CONTEXT", planRetrievedMemory),
		promptUserAnchor("end", claim.Job.Instruction, feedback),
		"Final grounding check: produce the plan for AUTHORITATIVE_USER_INSTRUCTION_END.",
	}, "\n\n")
	planPasses := planningPassCount(claim.Job)
	s.emitStepEvent(claim.Step.ID, "plan_strategy", fmt.Sprintf("candidates=%d", planPasses))
	candidates := make([]string, 0, planPasses)
	candidateSummaries := make([]string, 0, planPasses)
	candidateFailures := make([]string, 0, planPasses)
	for pass := 1; pass <= planPasses; pass++ {
		passPrompt := strings.Join([]string{
			plannerPrompt,
			promptBlock("PLANNING_PASS", fmt.Sprintf("%d/%d", pass, planPasses)),
			"Generate one independent plan candidate for this pass. Return only schema-valid JSON.",
		}, "\n\n")
		candidate, err := s.llmGenerateWithTrace(
			ctx,
			claim.Step.ID,
			fmt.Sprintf("plan_candidate_%d", pass),
			modelName,
			passPrompt,
		)
		if err != nil {
			candidateFailures = append(candidateFailures, fmt.Sprintf("pass %d request failed: %v", pass, err))
			s.emitStepEvent(claim.Step.ID, "plan_candidate_failed", fmt.Sprintf("pass=%d reason=%s", pass, trimForBudget(err.Error(), 180)))
			s.emitStepStream(claim.Step.ID, "stderr", fmt.Sprintf("plan candidate %d failed: %s", pass, trimForBudget(err.Error(), 300)))
			continue
		}
		candidate = normalizePlanText(candidate)
		if _, ok := parsePlanPayload(candidate); !ok || parsePlanTaskCount(candidate) == 0 {
			candidateFailures = append(candidateFailures, fmt.Sprintf("pass %d returned invalid or empty plan JSON", pass))
			s.emitStepEvent(claim.Step.ID, "plan_candidate_rejected", fmt.Sprintf("pass=%d reason=invalid_plan_json", pass))
			continue
		}
		if forceFreshExternal {
			candidate = forcePlanNeedsExternalInfo(candidate)
		}
		candidates = append(candidates, candidate)
		summary := summarizePlanCandidate(pass, candidate)
		candidateSummaries = append(candidateSummaries, summary)
		s.emitStepEvent(claim.Step.ID, "plan_candidate_ready", summary)
		s.emitStepStream(claim.Step.ID, "stdout", fmt.Sprintf("plan candidate %d generated chars=%d", pass, len(strings.TrimSpace(candidate))))
	}
	if len(candidates) == 0 {
		return fmt.Errorf("planner produced no valid candidates across %d pass(es): %s", planPasses, strings.Join(candidateFailures, "; "))
	}
	if len(candidateSummaries) > 0 {
		s.emitStepContext(claim.Step.ID, "plan_candidates", strings.Join(candidateSummaries, "\n"))
	}

	selectedIdx, selectionReason, err := s.selectBestPlanCandidateIndex(ctx, claim.Step.ID, claim.Job, modelName, feedback, actionCatalog, candidates, forceFreshExternal)
	if err != nil {
		return err
	}
	if selectedIdx < 0 || selectedIdx >= len(candidates) {
		return fmt.Errorf("planner candidate selection returned invalid index %d for %d candidates", selectedIdx, len(candidates))
	}
	plan := candidates[selectedIdx]
	s.emitStepEvent(claim.Step.ID, "plan_selected", fmt.Sprintf("candidate=%d/%d reason=%s", selectedIdx+1, len(candidates), trimForBudget(selectionReason, 260)))
	s.emitStepContext(claim.Step.ID, "plan_selection", strings.TrimSpace(strings.Join([]string{
		fmt.Sprintf("selected_candidate=%d", selectedIdx+1),
		"selection_reason=" + trimForBudget(selectionReason, 1200),
	}, "\n")))

	clarifications := planClarificationQuestions(plan, 3)
	if len(clarifications) > 0 {
		question := formatClarificationQuestions(clarifications)
		if (autonomy || persistent) && !mustAskForClarification(question, claim.Job.Instruction) {
			plan = clearPlanClarifications(plan)
		} else {
			output := "paused for clarification: " + question
			s.emitStepEvent(claim.Step.ID, "plan_waiting_input", fmt.Sprintf("clarifications=%d", len(clarifications)))
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"plan":                   plan,
				"clarification_required": "true",
			})
		}
	}

	needsExternal, _ := planNeedsExternalInfo(plan)
	s.emitStepEvent(claim.Step.ID, "plan_ready", fmt.Sprintf("tasks=%d needs_external=%t", parsePlanTaskCount(plan), needsExternal))
	return s.repo.CompleteStep(ctx, claim.Step.ID, plan, "plan", plan)
}

func (s *Service) runToolingStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	autonomy := autonomyEnabled(claim.Job)
	persistent := persistentExecutionEnabled(claim.Job)
	s.emitStepEvent(claim.Step.ID, "tooling_begin", fmt.Sprintf("autonomy=%s", resolveAutonomyMode(claim.Job)))
	hostSummary := buildHostEnvironmentSummaryFromMetadata(claim.Job)
	hostToolSet := hostToolSetFromMetadata(claim.Job)
	packageManagers := resolvePackageManagers(claim.Job)
	packageManager := primaryPackageManager(packageManagers)
	if autonomy && isFollowUpStatusCheckInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		summary := s.parentJobSummary(ctx, claim.Job)
		if strings.TrimSpace(summary) == "" {
			summary = "parent_job=unknown"
		}
		envSummary := buildEnvironmentSummary(packageManager, nil, nil, nil, s.workspace)
		if clientCWD := metadataString(claim.Job.Metadata, "client_cwd"); clientCWD != "" {
			envSummary = strings.TrimSpace(envSummary + "\nenv_client_cwd=" + clientCWD)
		}
		s.emitStepContext(claim.Step.ID, "environment", envSummary)
		if hostSummary != "" {
			s.emitStepContext(claim.Step.ID, "host_environment", hostSummary)
		}
		s.emitStepContext(claim.Step.ID, "parent_job", summary)
		output := strings.TrimSpace(strings.Join([]string{
			"required_tools=",
			"available_tools=",
			"host_available_tools=",
			"missing_tools=",
			"autonomy_mode=" + resolveAutonomyMode(claim.Job),
			summary,
			envSummary,
			hostSummary,
			"approval=not_required",
		}, "\n"))
		s.emitStepEvent(claim.Step.ID, "tooling_ready", "strategy=followup_status")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "tooling", output)
	}

	plan := contexts["plan"]
	requiredTools := parsePlanRequiredTools(plan)
	if len(requiredTools) == 0 {
		requiredTools = inferRequiredToolsFromInstruction(claim.Job.Instruction)
	}
	sort.Strings(requiredTools)

	if len(requiredTools) == 0 {
		envSummary := buildEnvironmentSummary(packageManager, nil, nil, nil, s.workspace)
		if clientCWD := metadataString(claim.Job.Metadata, "client_cwd"); clientCWD != "" {
			envSummary = strings.TrimSpace(envSummary + "\nenv_client_cwd=" + clientCWD)
		}
		s.emitStepContext(claim.Step.ID, "environment", envSummary)
		if hostSummary != "" {
			s.emitStepContext(claim.Step.ID, "host_environment", hostSummary)
		}
		output := strings.TrimSpace(strings.Join([]string{
			"no specific tool requirements inferred",
			"autonomy_mode=" + resolveAutonomyMode(claim.Job),
			envSummary,
			hostSummary,
		}, "\n"))
		s.emitStepEvent(claim.Step.ID, "tooling_ready", "required_tools=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "tooling", output)
	}
	s.emitStepEvent(claim.Step.ID, "tooling_requirements", fmt.Sprintf("required_tools=%d", len(requiredTools)))

	available := make([]string, 0, len(requiredTools))
	hostAvailable := make([]string, 0, len(requiredTools))
	missing := make([]string, 0, len(requiredTools))
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err == nil {
			available = append(available, tool)
			s.emitStepStream(claim.Step.ID, "stdout", "tool check: "+tool+" available")
		} else if hostToolAvailable(tool, hostToolSet) {
			hostAvailable = append(hostAvailable, tool)
			s.emitStepStream(claim.Step.ID, "stdout", "tool check: "+tool+" host-available")
		} else {
			missing = append(missing, tool)
			s.emitStepStream(claim.Step.ID, "stderr", "tool check: "+tool+" missing")
		}
	}
	sort.Strings(available)
	sort.Strings(hostAvailable)
	sort.Strings(missing)
	installHints := buildInstallHints(packageManagers, missing)
	installHint := ""
	if len(installHints) > 0 {
		installHint = installHints[0]
	}
	envSummary := buildEnvironmentSummary(packageManager, requiredTools, available, missing, s.workspace)
	if clientCWD := metadataString(claim.Job.Metadata, "client_cwd"); clientCWD != "" {
		envSummary = strings.TrimSpace(envSummary + "\nenv_client_cwd=" + clientCWD)
	}
	s.emitStepContext(claim.Step.ID, "environment", envSummary)
	if hostSummary != "" {
		s.emitStepContext(claim.Step.ID, "host_environment", hostSummary)
	}

	output := strings.TrimSpace(strings.Join([]string{
		"required_tools=" + strings.Join(requiredTools, ","),
		"available_tools=" + strings.Join(available, ","),
		"host_available_tools=" + strings.Join(hostAvailable, ","),
		"missing_tools=" + strings.Join(missing, ","),
		"package_manager=" + packageManager,
		"package_managers=" + strings.Join(packageManagers, ","),
		"install_hint=" + installHint,
		"install_hints=" + strings.Join(installHints, " || "),
		"autonomy_mode=" + resolveAutonomyMode(claim.Job),
		envSummary,
		hostSummary,
	}, "\n"))
	if output == "" {
		output = "tooling probe produced no output"
	}

	if len(missing) > 0 && !metadataBool(claim.Job.Metadata, "allow_missing_tools", false) {
		if (autonomy || persistent) && !mustAskForClarification(strings.Join(missing, ","), claim.Job.Instruction) {
			output = strings.TrimSpace(strings.Join([]string{
				output,
				"missing_tools_policy=auto_continue",
				"autonomy_note=missing tools detected; proceeding with best-effort assumptions.",
			}, "\n"))
		} else {
			question := "Missing tools: " + strings.Join(missing, ", ") + ". "
			if installHint != "" {
				question += "Install with `" + installHint + "` if appropriate, or provide alternatives. "
			} else {
				question += "Install them (or provide alternatives). "
			}
			question += "Submit feedback to continue."
			s.emitStepEvent(claim.Step.ID, "tooling_waiting_input", fmt.Sprintf("missing_tools=%d", len(missing)))
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"tooling":       output,
				"missing_tools": strings.Join(missing, ","),
				"install_hint":  installHint,
			})
		}
	}

	approvalMode := resolveApprovalMode(claim.Job.Metadata)
	riskSignals := detectRiskSignals(claim.Job.Instruction, plan)
	requireApproval := approvalMode == "force" || (approvalMode == "auto" && len(riskSignals) > 0)
	if persistent && approvalMode == "auto" && requireApproval && !mustAskForClarification(strings.Join(riskSignals, ","), claim.Job.Instruction) {
		requireApproval = false
		output = strings.TrimSpace(output + "\napproval=auto_bypassed_persistent\nrisk_signals=" + strings.Join(riskSignals, "|"))
	}
	if requireApproval {
		approvalFeedback := strings.TrimSpace(contexts["user_feedback"])
		if !hasExplicitApproval(approvalFeedback) {
			question := "Risk approval required before proceeding."
			if len(riskSignals) > 0 {
				question += " Signals: " + strings.Join(riskSignals, "; ") + "."
			}
			question += " Reply with `APPROVE: <notes>` to proceed or cancel the job."
			output = strings.TrimSpace(output + "\napproval=required\nrisk_signals=" + strings.Join(riskSignals, "|"))
			s.emitStepEvent(claim.Step.ID, "tooling_waiting_input", fmt.Sprintf("approval_required=true risk_signals=%d", len(riskSignals)))
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"tooling":           output,
				"approval_required": "true",
				"risk_signals":      strings.Join(riskSignals, ","),
			})
		}
		output = strings.TrimSpace(output + "\napproval=granted")
	} else {
		output = strings.TrimSpace(output + "\napproval=not_required")
	}

	s.emitStepEvent(claim.Step.ID, "tooling_ready", fmt.Sprintf("available=%d missing=%d", len(available), len(missing)))
	return s.repo.CompleteStep(ctx, claim.Step.ID, output, "tooling", output)
}

func (s *Service) runWorkspaceScanStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	mode := strings.ToLower(strings.TrimSpace(metadataString(claim.Job.Metadata, "workspace_scan")))
	persistent := persistentExecutionEnabled(claim.Job)
	if mode == "" {
		mode = "auto"
	}
	force := mode == "on" || mode == "force" || mode == "true"
	s.emitStepEvent(claim.Step.ID, "workspace_scan_begin", fmt.Sprintf("mode=%s force=%t", mode, force))
	if mode == "off" || mode == "false" {
		output := "workspace scan skipped: metadata mode=off"
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=mode_off")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}
	if !force && isDeterministicLocalActionReviewInstruction(claim.Job.Instruction) {
		output := "workspace scan skipped: deterministic local-action review does not require workspace search"
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=deterministic_local_action_review")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}
	if !force && isSimpleFileTaskInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		output := "workspace scan skipped: simple file creation request does not require workspace search"
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=simple_file_task")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}

	if !force && !shouldScanWorkspace(claim.Job.Instruction, contexts["plan"]) {
		output := "workspace scan skipped: not required for this instruction"
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=heuristic_not_required")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}

	if s.workspace == nil || !s.workspace.Enabled() {
		output := "workspace scan unavailable: service disabled"
		if force && !persistent {
			question := "Workspace scan is required for this task but currently unavailable. Set WORKSPACE_SCAN_ENABLED=true and WORKSPACE_ROOT, or submit feedback to continue without workspace context."
			s.emitStepEvent(claim.Step.ID, "workspace_scan_waiting_input", "reason=service_disabled")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"workspace": output,
			})
		}
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=service_disabled")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}

	snapshot, err := s.workspace.Snapshot()
	if err != nil {
		output := "workspace scan error: " + err.Error()
		if force && !persistent {
			question := "Workspace scan failed. Provide corrected workspace path/settings or submit feedback to continue without scan."
			s.emitStepEvent(claim.Step.ID, "workspace_scan_waiting_input", "reason=snapshot_error")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"workspace": output,
			})
		}
		s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=snapshot_error")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "workspace", output)
	}
	if force && strings.Contains(strings.ToLower(snapshot), "workspace_root not set") {
		if persistent {
			s.emitStepEvent(claim.Step.ID, "workspace_scan_skipped", "reason=workspace_root_missing_persistent")
			return s.repo.CompleteStep(ctx, claim.Step.ID, snapshot, "workspace", snapshot)
		}
		question := "Workspace scan is on but WORKSPACE_ROOT is not set. Set WORKSPACE_ROOT and retry, or submit feedback to continue without it."
		s.emitStepEvent(claim.Step.ID, "workspace_scan_waiting_input", "reason=workspace_root_missing")
		return s.repo.PauseStepForInput(ctx, claim.Step.ID, snapshot, question, map[string]string{
			"workspace": snapshot,
		})
	}

	snapshot = trimForBudget(snapshot, s.contextBudget)
	if strings.TrimSpace(snapshot) == "" {
		snapshot = "workspace scan produced no output"
	}
	s.emitStepEvent(claim.Step.ID, "workspace_scan_ready", fmt.Sprintf("snapshot_chars=%d", len(strings.TrimSpace(snapshot))))
	return s.repo.CompleteStep(ctx, claim.Step.ID, snapshot, "workspace", snapshot)
}
