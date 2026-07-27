package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/chat"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"strings"
	"time"
)

func (s *Service) runTagStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	s.emitStepEvent(claim.Step.ID, "tag_begin", fmt.Sprintf("instruction_chars=%d", len(strings.TrimSpace(claim.Job.Instruction))))

	tagDefault := s.specialistModel(claim.Job, specialist.RoleIntentTaggingSpecialist, s.models.Tagging)
	tagModel := metadataModel(claim.Job, "model_tagger", tagDefault)
	if strings.TrimSpace(tagModel) == "" {
		return fmt.Errorf("tagging model is not configured")
	}
	s.emitStepEvent(claim.Step.ID, "tag_model", "model="+tagModel)
	tagInput := strings.TrimSpace(claim.Job.Instruction)
	plan := trimForBudget(contexts["plan"], 1200)
	if plan != "" {
		tagInput = strings.TrimSpace(tagInput + "\n\nPlan:\n" + plan)
	}
	tags, err := s.llm.SuggestTagsWithModel(ctx, tagModel, tagInput, 8)
	if err != nil {
		return fmt.Errorf("tag specialist failed with configured model %q: %w", tagModel, err)
	}
	if len(tags) == 0 {
		return fmt.Errorf("tag specialist returned no tags with configured model %q", tagModel)
	}

	output := strings.Join(tags, ",")
	if output == "" {
		output = "general"
	}

	s.emitStepEvent(claim.Step.ID, "tag_ready", fmt.Sprintf("tags=%d", len(parseTagsCSV(output))))
	return s.repo.CompleteStep(ctx, claim.Step.ID, output, "tags", output)
}

func (s *Service) runRetrieveStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	s.emitStepEvent(claim.Step.ID, "retrieve_begin", fmt.Sprintf("instruction_chars=%d", len(strings.TrimSpace(claim.Job.Instruction))))
	if isDeterministicLocalActionReviewInstruction(claim.Job.Instruction) {
		output := "Historical memory retrieval skipped: deterministic local-action review relies on immediate execution evidence."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=deterministic_local_action_skip matches=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}
	if isLowSignalChatInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		output := "No relevant memory needed for brief conversational input."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=low_signal matches=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}
	if autonomyEnabled(claim.Job) && isFollowUpStatusCheckInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		output := "No retrieval needed: use parent job result/context for completion follow-up."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=followup_status matches=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}
	if autonomyEnabled(claim.Job) && isSimpleFileTaskInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		output := "No retrieval needed: simple file/document task should use local environment defaults."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=simple_file_task matches=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}
	if shouldBypassHistoricalContext(claim.Job.Instruction, contexts["user_feedback"]) {
		output := "Historical memory retrieval skipped: fresh context requested for this turn."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=fresh_context_skip matches=0")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}
	if shouldRetrieve, reason := shouldRetrieveHistoricalMemory(claim.Job, contexts); !shouldRetrieve {
		output := "Historical memory retrieval skipped: " + reason + "."
		s.emitStepEvent(claim.Step.ID, "retrieve_ready", "strategy=light_memory_skip matches=0 reason="+safeLine(reason, "skip"))
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
	}

	tags := memoryScopeTags(claim.Job, parseTagsCSV(contexts["tags"]))
	projectScope := projectTag(claim.Job)
	sessionScope := sessionTag(claim.Job)
	retrievalLimit := resolveMemoryRetrievalLimit(claim.Job, claim.Job.Instruction, contexts["user_feedback"], s.retrievalLimit)
	candidateLimit := resolveMemoryCandidateLimit(retrievalLimit)
	s.emitStepEvent(claim.Step.ID, "retrieve_limit", fmt.Sprintf("limit=%d candidates=%d", retrievalLimit, candidateLimit))
	embedding, err := s.llm.Embedding(ctx, claim.Job.Instruction)
	if err != nil {
		s.emitStepStream(claim.Step.ID, "stderr", "retrieval embedding failed: "+trimForBudget(err.Error(), 260))
		return fmt.Errorf("memory retrieval embedding failed: %w", err)
	}

	scopeMode := "global"
	initialQueryTags := tags
	var matches []model.MemoryMatch
	if projectScope != "" {
		scopeMode = "project"
		initialQueryTags = []string{projectScope}
		matches, err = s.repo.FindRelevantMemory(ctx, embedding, initialQueryTags, candidateLimit)
		if err != nil {
			return err
		}
	} else {
		matches, err = s.repo.FindRelevantMemory(ctx, embedding, initialQueryTags, candidateLimit)
		if err != nil {
			return err
		}
	}

	relatedTags := deriveRelatedMemoryTags(tags, matches, maxRelatedMemoryTags)
	omnibus := append([]model.MemoryMatch{}, matches...)
	if scopeMode != "project" {
		expandedTags := appendUnique(initialQueryTags, tags...)
		expandedTags = appendUnique(expandedTags, relatedTags...)
		if !sameTagSet(expandedTags, initialQueryTags) {
			relatedMatches, relErr := s.repo.FindRelevantMemory(ctx, embedding, expandedTags, candidateLimit)
			if relErr != nil {
				return relErr
			}
			omnibus = mergeMemoryMatches(omnibus, relatedMatches)
		}
	}

	ranked := rankMemoryOmnibusMatches(
		omnibus,
		claim.Job.Instruction,
		tags,
		projectScope,
		sessionScope,
		retrievalLimit,
		time.Now().UTC(),
	)

	output := buildRetrievalContext(ranked, s.contextBudget)
	if strings.TrimSpace(output) == "" {
		output = "No relevant memory found."
	}
	if len(relatedTags) > 0 {
		output = strings.TrimSpace(strings.Join([]string{
			"retrieval_related_tags=" + strings.Join(relatedTags, "|"),
			output,
		}, "\n"))
	}
	if projectScope != "" {
		output = strings.TrimSpace(strings.Join([]string{
			"retrieval_scope=" + scopeMode,
			"project_tag=" + projectScope,
			output,
		}, "\n"))
	}

	s.emitStepEvent(
		claim.Step.ID,
		"retrieve_ready",
		fmt.Sprintf("matches=%d candidates=%d related_tags=%d output_chars=%d", len(ranked), len(omnibus), len(relatedTags), len(strings.TrimSpace(output))),
	)
	return s.repo.CompleteStep(ctx, claim.Step.ID, output, "retrieval", output)
}

func resolveMemoryRetrievalLimit(job model.Job, instruction string, feedback string, fallback int) int {
	limit := fallback
	if limit < 1 {
		limit = 8
	}
	if limit > maxMemoryRetrievalLimit {
		limit = maxMemoryRetrievalLimit
	}

	for _, key := range []string{"retrieval_limit", "memory_retrieval_limit", "memory_lookback_limit"} {
		value := metadataInt(job.Metadata, key, 0)
		if value <= 0 {
			continue
		}
		if value > maxMemoryRetrievalLimit {
			return maxMemoryRetrievalLimit
		}
		return value
	}

	lookbackMode := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, "memory_lookback")))
	switch lookbackMode {
	case "deep", "full", "historical", "all":
		target := limit * 3
		if target < limit+6 {
			target = limit + 6
		}
		return minInt(maxMemoryRetrievalLimit, target)
	}

	if shouldDeepenMemoryLookback(instruction, feedback) {
		target := limit * 3
		if target < limit+6 {
			target = limit + 6
		}
		return minInt(maxMemoryRetrievalLimit, target)
	}
	return limit
}

func shouldDeepenMemoryLookback(instruction string, feedback string) bool {
	text := strings.TrimSpace(strings.Join([]string{instruction, feedback}, "\n"))
	if text == "" {
		return false
	}
	if memoryLookbackPattern.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "think back") ||
		strings.Contains(lower, "look back") ||
		strings.Contains(lower, "older memory") ||
		strings.Contains(lower, "earlier memory")
}

func shouldRetrieveHistoricalMemory(job model.Job, contexts map[string]string) (bool, string) {
	mode := resolveHistoricalMemoryMode(job.Metadata)
	switch mode {
	case "on":
		return true, "forced on by metadata"
	case "off":
		return false, "disabled by metadata"
	}

	if strings.ToLower(strings.TrimSpace(job.Pipeline)) != model.PipelineChat {
		return true, "enabled for non-chat pipeline"
	}

	feedback := strings.TrimSpace(contexts["user_feedback"])
	if shouldBypassHistoricalContext(job.Instruction, feedback) {
		return false, "fresh context requested"
	}
	if shouldDeepenMemoryLookback(job.Instruction, feedback) || explicitHistoricalRecallPattern.MatchString(strings.ToLower(strings.TrimSpace(strings.Join([]string{job.Instruction, feedback}, "\n")))) {
		return true, "explicit historical recall requested"
	}

	return false, "light chat mode (recent conversation handles short references)"
}

func resolveHistoricalMemoryMode(metadata json.RawMessage) string {
	for _, key := range []string{"memory_retrieval", "historical_memory", "memory_mode"} {
		value, ok := metadataValue(metadata, key)
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return "on"
			}
			return "off"
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "on", "always", "deep", "full", "all":
				return "on"
			case "off", "none", "light", "shallow", "recent_only", "recent-only":
				return "off"
			case "auto", "":
				return "auto"
			}
		}
	}
	return "auto"
}

func (s *Service) runAnalyzeStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	s.emitStepEvent(claim.Step.ID, "analyze_begin", fmt.Sprintf("autonomy=%s", resolveAutonomyMode(claim.Job)))
	if isLowSignalChatInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		analysis := strings.Join([]string{
			"- Brief conversational check-in detected.",
			"- Respond directly and concisely.",
			"- Do not block on NEED_INPUT for this turn.",
		}, "\n")
		s.emitStepEvent(claim.Step.ID, "analyze_ready", "strategy=low_signal")
		return s.repo.CompleteStep(ctx, claim.Step.ID, analysis, "analyzer", analysis)
	}

	autonomy := autonomyEnabled(claim.Job)
	persistent := persistentExecutionEnabled(claim.Job)
	if autonomy && isFollowUpStatusCheckInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		parent := strings.TrimSpace(contexts["parent_job"])
		if parent == "" {
			parent = "parent_job=unknown"
		}
		analysis := strings.Join([]string{
			"- Follow-up completion check detected.",
			"- Answer directly from parent job status/result.",
			"- Do not ask clarifying questions for this turn.",
			"- " + parent,
		}, "\n")
		s.emitStepEvent(claim.Step.ID, "analyze_ready", "strategy=followup_status")
		return s.repo.CompleteStep(ctx, claim.Step.ID, analysis, "analyzer", analysis)
	}

	analyzeFallback := s.specialistModel(claim.Job, specialist.RoleAnalysisSpecialist, s.models.Analyze)
	analysisModel := s.pickThinkingModel(claim.Job, contexts, metadataModel(claim.Job, "model_analyze", analyzeFallback))
	s.emitStepEvent(claim.Step.ID, "analyze_model", "model="+analysisModel)

	goal := strings.TrimSpace(claim.Job.Instruction)
	if goal == "" {
		goal = "analyze user request and produce grounded execution guidance"
	}
	preparedContexts, err := s.prepareTournamentContexts(ctx, claim.Step.ID, analysisModel, goal, []tournamentContextRequest{
		{SourceKey: "plan", Value: contexts["plan"], Budget: s.contextBudget},
		{SourceKey: "tooling", Value: contexts["tooling"], Budget: s.contextBudget},
		{SourceKey: "recent_conversation", Value: contexts["recent_conversation"], Budget: 1800},
		{SourceKey: "retrieval", Value: contexts["retrieval"], Budget: s.contextBudget},
		{SourceKey: "workspace", Value: contexts["workspace"], Budget: s.contextBudget},
		{SourceKey: "web_search", Value: contexts["web_search"], Budget: s.contextBudget},
	})
	if err != nil {
		return err
	}
	plan := preparedContexts["plan"]
	tooling := preparedContexts["tooling"]
	environment := trimForBudget(contexts["environment"], 1200)
	recentConversation := preparedContexts["recent_conversation"]
	retrieval := preparedContexts["retrieval"]
	workspaceContext := preparedContexts["workspace"]
	web := preparedContexts["web_search"]
	feedback := trimForBudget(contexts["user_feedback"], 1200)
	tags := contexts["tags"]

	prompt := strings.Join([]string{
		"You are an analyzer. Summarize only what matters for a response.",
		antiRoleplayInstructionForPipeline(claim.Job.Pipeline),
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", claim.Job.Instruction, feedback),
		"Output plain text only.",
		"Keep it short: 6 bullet points max.",
		"Context precedence:",
		"1) USER_INSTRUCTION and USER_FEEDBACK are authoritative.",
		"2) RECENT_CONVERSATION is the immediate same-session history.",
		"3) TOOLING and WORKSPACE describe current-run facts.",
		"4) RETRIEVED_MEMORY is historical context and may be stale/hypothetical.",
		"5) WEB_SEARCH may be partial or noisy.",
		promptBlock("CURRENT_TIME_CONTEXT", currentTimeContextFromMetadata(claim.Job)),
		promptBlock("USER_INSTRUCTION", claim.Job.Instruction),
		promptBlock("USER_FEEDBACK", feedback),
		promptBlock("RECENT_CONVERSATION", recentConversation),
		promptBlock("PLAN", plan),
		promptBlock("TOOLING", tooling),
		promptBlock("ENVIRONMENT", environment),
		promptBlock("TAGS", tags),
		promptBlock("WORKSPACE", workspaceContext),
		promptBlock("RETRIEVED_MEMORY", retrieval),
		promptBlock("WEB_SEARCH", web),
		"Rules: do not invent facts.",
		"Do not treat RETRIEVED_MEMORY as proof that commands were executed in this run.",
		"If AUTONOMY_MODE is on, infer sensible defaults from TOOLING/ENVIRONMENT and avoid NEED_INPUT unless safety-critical.",
		promptBlock("AUTONOMY_MODE", resolveAutonomyMode(claim.Job)),
		promptUserAnchor("end", claim.Job.Instruction, feedback),
		"Final grounding check: every bullet must stay aligned with AUTHORITATIVE_USER_INSTRUCTION_END.",
		"If critical info is missing, start with NEED_INPUT: followed by one concise question.",
	}, "\n\n")

	analysis, err := s.llmGenerateWithTrace(ctx, claim.Step.ID, "analyze", analysisModel, prompt)
	if err != nil {
		return err
	}

	analysis = trimForBudget(analysis, s.contextBudget)
	if question, ok := extractNeedInputQuestion(analysis); ok {
		if (autonomy || persistent) && !mustAskForClarification(question, claim.Job.Instruction) {
			analysis = strings.Join([]string{
				"- Autonomous mode: proceeding with inferred defaults from environment/tooling context.",
				"- Missing detail was non-blocking and not safety-critical.",
				"- Final response should state assumptions briefly and continue.",
			}, "\n")
		} else {
			s.emitStepEvent(claim.Step.ID, "analyze_waiting_input", "need_input=true")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, analysis, question, map[string]string{
				"analyzer": analysis,
			})
		}
	}
	s.emitStepEvent(claim.Step.ID, "analyze_ready", fmt.Sprintf("analysis_chars=%d", len(strings.TrimSpace(analysis))))
	return s.repo.CompleteStep(ctx, claim.Step.ID, analysis, "analyzer", analysis)
}

func (s *Service) runResponseStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	styleInstruction := map[string]string{
		"assist":   "You are a direct assistant. Answer clearly and concretely.",
		"roleplay": "You are a direct assistant in a real-world session. Do not roleplay or adopt fictional personas.",
		"narrate":  "You are a narrator. Continue the scene with concise progression.",
	}[action]
	s.emitStepEvent(claim.Step.ID, "response_begin", fmt.Sprintf("action=%s autonomy=%s", action, resolveAutonomyMode(claim.Job)))
	lowSignalChat := isLowSignalChatInstruction(claim.Job.Instruction, claim.Job.Pipeline)
	autonomy := autonomyEnabled(claim.Job)
	persistent := persistentExecutionEnabled(claim.Job)
	if autonomy && isFollowUpStatusCheckInstruction(claim.Job.Instruction, claim.Job.Pipeline) {
		response := s.followUpStatusResponse(ctx, claim.Job)
		response = ensureResponseHasSources(response, claim.Job, contexts, nil)
		s.emitStepEvent(claim.Step.ID, "response_ready", "strategy=followup_status")
		if err := s.repo.CompleteStep(ctx, claim.Step.ID, response, action, response); err != nil {
			return err
		}
		if err := s.persistMemory(ctx, claim.Job, contexts, response); err != nil {
			s.logger.Printf("job=%d memory persist warning: %v", claim.Job.ID, err)
		}
		if err := s.inferMemory(ctx, claim.Step.ID, claim.Job, contexts, response); err != nil {
			s.logger.Printf("job=%d inferred memory warning: %v", claim.Job.ID, err)
		}
		return nil
	}
	if lowSignalChat && strings.TrimSpace(contexts["user_feedback"]) == "" {
		response := chat.LowSignalResponse(claim.Job.Instruction)
		response = ensureResponseHasSources(response, claim.Job, contexts, nil)
		s.emitStepEvent(claim.Step.ID, "response_ready", "strategy=low_signal")
		if err := s.repo.CompleteStep(ctx, claim.Step.ID, response, action, response); err != nil {
			return err
		}
		if err := s.persistMemory(ctx, claim.Job, contexts, response); err != nil {
			s.logger.Printf("job=%d memory persist warning: %v", claim.Job.ID, err)
		}
		if err := s.inferMemory(ctx, claim.Step.ID, claim.Job, contexts, response); err != nil {
			s.logger.Printf("job=%d inferred memory warning: %v", claim.Job.ID, err)
		}
		return nil
	}

	responseFallback := s.specialistModel(claim.Job, specialist.RoleResponseSpecialist, s.models.Response)
	responseModel := s.pickThinkingModel(claim.Job, contexts, metadataModel(claim.Job, "model_response", responseFallback))
	codeOnly := shouldForceCodeOnlyResponse(claim.Job, contexts, responseModel)
	s.emitStepEvent(claim.Step.ID, "response_model", "model="+responseModel)
	if codeOnly {
		s.emitStepEvent(claim.Step.ID, "response_mode", "mode=code_only")
	}

	goal := strings.TrimSpace(claim.Job.Instruction)
	if goal == "" {
		goal = "answer user request with grounded, verifiable output"
	}
	preparedContexts, err := s.prepareTournamentContexts(ctx, claim.Step.ID, responseModel, goal, []tournamentContextRequest{
		{SourceKey: "retrieval", Value: contexts["retrieval"], Budget: s.contextBudget},
		{SourceKey: "analyzer", Value: contexts["analyzer"], Budget: s.contextBudget},
		{SourceKey: "plan", Value: contexts["plan"], Budget: s.contextBudget},
		{SourceKey: "tooling", Value: contexts["tooling"], Budget: s.contextBudget},
		{SourceKey: "recent_conversation", Value: contexts["recent_conversation"], Budget: 1800},
		{SourceKey: "workspace", Value: contexts["workspace"], Budget: s.contextBudget},
		{SourceKey: "web_search", Value: contexts["web_search"], Budget: s.contextBudget},
	})
	if err != nil {
		return err
	}
	retrieval := preparedContexts["retrieval"]
	analysis := preparedContexts["analyzer"]
	plan := preparedContexts["plan"]
	tooling := preparedContexts["tooling"]
	environment := trimForBudget(contexts["environment"], 1200)
	recentConversation := preparedContexts["recent_conversation"]
	workspaceContext := preparedContexts["workspace"]
	web := preparedContexts["web_search"]
	feedback := trimForBudget(contexts["user_feedback"], 1200)
	prompt := strings.Join([]string{
		styleInstruction,
		antiRoleplayInstructionForPipeline(claim.Job.Pipeline),
		"Treat this as a fresh thread. Do not rely on hidden prior context.",
		"Use only the explicit blocks below.",
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", claim.Job.Instruction, feedback),
		"Context precedence:",
		"1) USER_INSTRUCTION and USER_FEEDBACK.",
		"2) RECENT_CONVERSATION from the same chat session.",
		"3) ANALYZER + TOOLING + WORKSPACE from this run.",
		"4) RETRIEVED_MEMORY as historical context only.",
		"5) WEB_SEARCH.",
		promptBlock("CURRENT_TIME_CONTEXT", currentTimeContextFromMetadata(claim.Job)),
		promptBlock("USER_INSTRUCTION", claim.Job.Instruction),
		promptBlock("USER_FEEDBACK", feedback),
		promptBlock("RECENT_CONVERSATION", recentConversation),
		promptBlock("PLAN", plan),
		promptBlock("TOOLING", tooling),
		promptBlock("ENVIRONMENT", environment),
		promptBlock("ANALYZER", analysis),
		promptBlock("WORKSPACE", workspaceContext),
		promptBlock("RETRIEVED_MEMORY", retrieval),
		promptBlock("WEB_SEARCH", web),
		promptBlock("AUTONOMY_MODE", resolveAutonomyMode(claim.Job)),
		"Rules: if a blocking unknown remains, start the response with NEED_INPUT: followed by one concise question.",
		"For brief check-ins like hello/test/ping, respond directly in one short sentence and do not use NEED_INPUT.",
		"Do not claim commands/tests/deployments happened unless TOOLING/WORKSPACE/WEB_SEARCH in this run explicitly supports it.",
		"If AUTONOMY_MODE is on, make sensible defaults and continue without asking follow-up questions unless safety-critical.",
		func() string {
			if !codeOnly {
				return ""
			}
			return "OUTPUT_MODE=CODE_ONLY. Return only raw file/code contents with no markdown fences, backticks, explanations, headings, or source blocks."
		}(),
		promptUserAnchor("end", claim.Job.Instruction, feedback),
		"Final grounding check: the final answer must satisfy AUTHORITATIVE_USER_INSTRUCTION_END.",
	}, "\n\n")

	response, err := s.llmGenerateWithTrace(ctx, claim.Step.ID, "response_draft", responseModel, prompt)
	if err != nil {
		return err
	}

	response = strings.TrimSpace(response)
	if question, ok := extractNeedInputQuestion(response); ok {
		if lowSignalChat {
			response = "I'm here and ready. Tell me what you'd like to work on."
		} else if (autonomy || persistent) && !mustAskForClarification(question, claim.Job.Instruction) {
			response = s.rewriteNeedInputAutonomous(ctx, claim.Step.ID, claim.Job, contexts, response, question)
		} else {
			s.emitStepEvent(claim.Step.ID, "response_waiting_input", "need_input=true")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, response, question, map[string]string{
				"response_draft": response,
			})
		}
	}
	if strings.TrimSpace(response) == "" && lowSignalChat {
		response = "I'm here and ready. Tell me what you'd like to work on."
	}
	if question, ok := extractNeedInputQuestion(response); ok {
		s.emitStepEvent(claim.Step.ID, "response_waiting_input", "need_input=true")
		return s.repo.PauseStepForInput(ctx, claim.Step.ID, response, question, map[string]string{
			"response_draft": response,
		})
	}
	if codeOnly {
		response = normalizeCodeOnlyResponse(response)
	} else {
		response = ensureResponseHasSources(response, claim.Job, contexts, nil)
	}
	s.emitStepEvent(claim.Step.ID, "response_ready", fmt.Sprintf("response_chars=%d", len(strings.TrimSpace(response))))
	if err := s.repo.CompleteStep(ctx, claim.Step.ID, response, action, response); err != nil {
		return err
	}

	if err := s.memorizeSuccessfulJob(ctx, claim.Job.ID); err != nil {
		s.logger.Printf("job=%d success playbook memory warning: %v", claim.Job.ID, err)
	}
	if err := s.persistMemory(ctx, claim.Job, contexts, response); err != nil {
		s.logger.Printf("job=%d memory persist warning: %v", claim.Job.ID, err)
	}
	if err := s.inferMemory(ctx, claim.Step.ID, claim.Job, contexts, response); err != nil {
		s.logger.Printf("job=%d inferred memory warning: %v", claim.Job.ID, err)
	}

	return nil
}
