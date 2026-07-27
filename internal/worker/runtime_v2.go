package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/websearch"
)

type agentRuntime struct {
	svc      *Service
	ctx      context.Context
	claim    *model.ClaimedStep
	action   string
	contexts map[string]string
}

func (s *Service) runAgentRuntimeStep(
	ctx context.Context,
	claim *model.ClaimedStep,
	contexts map[string]string,
	action string,
) error {
	rt := &agentRuntime{
		svc:      s,
		ctx:      ctx,
		claim:    claim,
		action:   action,
		contexts: contexts,
	}
	return rt.run()
}

func (r *agentRuntime) run() error {
	switch r.action {
	case "tooling":
		return r.runTooling()
	case "workspace_scan":
		return r.runWorkspace()
	case "tag":
		return r.runTagging()
	case "retrieve":
		return r.runRetrieval()
	case "plan":
		return r.runPlanning()
	case "web_search":
		return r.runWebSearch()
	case "analyze":
		return r.runAnalyze()
	case "assist", "roleplay", "narrate":
		return r.runResponse()
	case "verify":
		return r.runVerify()
	default:
		return fmt.Errorf("unsupported agent runtime action %q", r.action)
	}
}

func (r *agentRuntime) runTooling() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "tooling_begin", "runtime=v2")
	_ = r.ensureV3IntentArtifact()

	hostTools := metadataCSV(r.claim.Job.Metadata, "host_tools_available")
	packageManagers := resolvePackageManagers(r.claim.Job)
	requiredTools := r.inferRequiredTools()
	missingTools := r.missingTools(requiredTools, hostTools)
	hints := buildInstallHints(packageManagers, missingTools)
	allowedTools := make([]string, 0, len(requiredTools))
	missingSet := make(map[string]struct{}, len(missingTools))
	for _, tool := range missingTools {
		missingSet[tool] = struct{}{}
	}
	for _, tool := range requiredTools {
		if _, missing := missingSet[tool]; missing {
			continue
		}
		allowedTools = append(allowedTools, tool)
	}

	lines := []string{
		"runtime=v2",
		"tooling_status=ok",
		fmt.Sprintf("host_tools=%s", csvOrNone(hostTools)),
		fmt.Sprintf("required_tools=%s", csvOrNone(requiredTools)),
		fmt.Sprintf("missing_tools=%s", csvOrNone(missingTools)),
		fmt.Sprintf("package_managers=%s", csvOrNone(packageManagers)),
	}
	if len(hints) > 0 {
		lines = append(lines, "install_hints:")
		lines = append(lines, hints...)
	}

	if r.svc.v3Active() {
		audit := artifacts.CapabilityAuditArtifact{
			AllowedTools: allowedTools,
			MissingTools: missingTools,
			WorkspaceOK:  r.svc.workspace != nil && r.svc.workspace.Enabled(),
			WebSearchOK:  r.svc.webSearch != nil,
			Notes: []string{
				"tool inventory derived from job metadata and capability heuristics",
				"tooling no longer depends on the planning step to determine missing capabilities",
			},
		}
		if err := r.writeArtifact(artifacts.KindCapabilityAudit, audit); err != nil {
			return err
		}
		if err := r.writeEvidence(evidence.Record{
			JobID:      r.claim.Job.ID,
			StepID:     r.claim.Step.ID,
			Kind:       evidence.KindModelJudgment,
			SourceType: "runtime_v2",
			SourceRef:  "capability_audit",
			Summary:    "Capability audit captured available, required, and missing tools for planning.",
			Confidence: 0.93,
			Metadata: map[string]any{
				"host_tools":       hostTools,
				"required_tools":   requiredTools,
				"missing_tools":    missingTools,
				"package_managers": packageManagers,
			},
		}); err != nil {
			return err
		}
	}

	output := strings.Join(lines, "\n")
	r.svc.emitStepEvent(r.claim.Step.ID, "tooling_complete", fmt.Sprintf("missing_tools=%d", len(missingTools)))
	return r.complete("tooling", output, output)
}

func (r *agentRuntime) runWorkspace() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "workspace_scan_begin", "runtime=v2")
	mode := strings.ToLower(strings.TrimSpace(metadataString(r.claim.Job.Metadata, "workspace_scan")))
	if mode == "off" || mode == "false" || mode == "disabled" {
		output := "workspace scan skipped: metadata mode=off"
		return r.complete("workspace", output, output)
	}
	if r.svc.workspace == nil || !r.svc.workspace.Enabled() {
		output := "workspace scan skipped: service disabled"
		return r.complete("workspace", output, output)
	}

	query := strings.TrimSpace(strings.Join([]string{r.claim.Job.Instruction, r.contexts["user_feedback"]}, "\n"))
	research, err := r.svc.workspace.Research(query)
	if err != nil {
		return err
	}
	output := strings.TrimSpace(research.Context)
	if output == "" {
		output, err = r.svc.workspace.Snapshot()
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(output) == "" {
		output = "workspace scan produced no output"
	}

	if r.svc.v3Active() {
		artifact := artifacts.WorkspaceArtifact{
			Root:            research.Root,
			FilesConsidered: research.FilesConsidered,
			Summary:         strings.TrimSpace(research.Summary),
			RelevantFiles:   mapWorkspaceExcerpts(research.Excerpts),
			Languages:       append([]string(nil), research.Languages...),
		}
		if err := r.writeArtifact(artifacts.KindWorkspace, artifact); err != nil {
			return err
		}
		for _, excerpt := range research.Excerpts {
			if err := r.writeEvidence(evidence.Record{
				JobID:      r.claim.Job.ID,
				StepID:     r.claim.Step.ID,
				Kind:       evidence.KindFileExcerpt,
				SourceType: "workspace",
				SourceRef:  excerpt.Path,
				FilePaths:  []string{excerpt.Path},
				Excerpt:    excerpt.Excerpt,
				Summary:    excerpt.Reason,
				Confidence: excerpt.Score,
			}); err != nil {
				return err
			}
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "workspace_scan_complete", fmt.Sprintf("chars=%d", len(output)))
	return r.complete("workspace", output, output)
}

func (r *agentRuntime) runTagging() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "tag_begin", "runtime=v2")

	modelName := r.svc.specialistModel(r.claim.Job, specialist.RoleIntentTaggingSpecialist, r.svc.models.Tagging)
	tags, err := r.svc.llm.SuggestTagsWithModel(r.ctx, modelName, r.claim.Job.Instruction, 8)
	if err != nil || len(tags) == 0 {
		tags = []string{"general"}
	}

	tags = memoryScopeTags(r.claim.Job, tags)
	if len(tags) == 0 {
		tags = memoryScopeTags(r.claim.Job, []string{"general"})
	}
	output := strings.Join(tags, ", ")

	r.svc.emitStepEvent(r.claim.Step.ID, "tag_complete", fmt.Sprintf("tags=%d", len(tags)))
	return r.complete("tags", output, output)
}

func (r *agentRuntime) runRetrieval() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "retrieve_begin", "runtime=v2")

	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	feedback := strings.TrimSpace(r.contexts["user_feedback"])
	retrieveHistorical, reason := shouldRetrieveHistoricalMemory(r.claim.Job, r.contexts)
	if !retrieveHistorical {
		output := strings.TrimSpace("Historical memory retrieval skipped: " + reason)
		if output == "Historical memory retrieval skipped:" {
			output = "Historical memory retrieval skipped: retrieval not required for this turn."
		}
		return r.complete("retrieval", output, output)
	}

	limit := resolveMemoryRetrievalLimit(r.claim.Job, instruction, feedback, r.svc.retrievalLimit)
	content := strings.TrimSpace(strings.Join([]string{instruction, feedback}, "\n"))
	var embedding []float64
	if content != "" {
		embedModel := r.svc.specialistModel(r.claim.Job, specialist.RoleMemoryRetrievalSpecialist, r.svc.models.Memory)
		r.svc.emitStepEvent(r.claim.Step.ID, "retrieve_embedding", fmt.Sprintf("model=%s", safeLine(embedModel, "unknown")))
		value, err := r.svc.llm.Embedding(r.ctx, content)
		if err == nil {
			embedding = value
		} else {
			r.svc.emitStepEvent(r.claim.Step.ID, "retrieve_embedding_error", trimForBudget(err.Error(), 180))
		}
	}

	scopeTags := splitCSVTags(r.contexts["tags"])
	scopeTags = memoryScopeTags(r.claim.Job, scopeTags)
	matches, err := r.svc.repo.FindRelevantMemory(r.ctx, embedding, scopeTags, limit)
	if err != nil && len(embedding) > 0 {
		matches, err = r.svc.repo.FindRelevantMemory(r.ctx, nil, scopeTags, limit)
	}
	if err != nil {
		return err
	}

	sessionID := metadataString(r.claim.Job.Metadata, "session_id")
	sessionTag := ""
	if sessionID != "" {
		sessionTag = "session:" + sessionID
	}
	projectScope := projectTag(r.claim.Job)
	ranked := rankMemoryOmnibusMatches(
		matches,
		strings.TrimSpace(instruction+" "+feedback),
		scopeTags,
		projectScope,
		sessionTag,
		limit,
		time.Now().UTC(),
	)

	relatedTags := deriveRelatedMemoryTags(scopeTags, ranked, maxRelatedMemoryTags)
	if len(relatedTags) > 0 {
		r.svc.emitStepContext(r.claim.Step.ID, "related_memory_tags", strings.Join(relatedTags, ", "))
	}

	output := buildRetrievalContext(ranked, r.svc.contextBudget)
	if strings.TrimSpace(output) == "" {
		output = "No relevant memory matched for this step."
	}
	if r.svc.v3Active() {
		items := make([]artifacts.RetrievalItem, 0, len(ranked))
		for idx, match := range ranked {
			items = append(items, artifacts.RetrievalItem{ID: match.ID, Kind: match.Kind, Content: match.Content, Tags: match.Tags, Score: match.Score})
			if idx < 8 {
				if err := r.writeEvidence(evidence.Record{
					JobID:      r.claim.Job.ID,
					StepID:     r.claim.Step.ID,
					Kind:       evidence.KindMemoryExcerpt,
					SourceType: "memory",
					SourceRef:  fmt.Sprintf("memory:%d", match.ID),
					Excerpt:    trimForBudget(match.Content, 800),
					Summary:    match.Kind,
					Confidence: match.Score,
					Metadata:   map[string]any{"tags": match.Tags},
				}); err != nil {
					return err
				}
			}
		}
		if err := r.writeArtifact(artifacts.KindRetrieval, artifacts.RetrievalArtifact{Summary: output, Items: items}); err != nil {
			return err
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "retrieve_complete", fmt.Sprintf("matches=%d", len(ranked)))
	return r.complete("retrieval", output, output)
}

func (r *agentRuntime) runPlanning() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "plan_begin", "runtime=v2")
	_ = r.ensureV3IntentArtifact()

	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	forceFreshExternal := shouldForceFreshWebSearch(instruction, r.contexts["user_feedback"])
	planPasses := planningPassCount(r.claim.Job)
	modelName := r.svc.specialistModel(r.claim.Job, specialist.RolePlannerSpecialist, r.svc.models.Plan)

	candidates := make([]artifacts.PlanArtifact, 0, planPasses)
	candidateFailures := make([]string, 0, planPasses)
	for i := 1; i <= planPasses; i++ {
		scope := fmt.Sprintf("plan_candidate_%d", i)
		prompt := r.planPrompt(forceFreshExternal, i, planPasses)
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, scope, modelName, prompt)
		if err != nil {
			r.svc.emitStepEvent(r.claim.Step.ID, "plan_candidate_error", fmt.Sprintf("index=%d error=%s", i, trimForBudget(err.Error(), 180)))
			candidateFailures = append(candidateFailures, fmt.Sprintf("candidate %d request failed: %v", i, err))
			continue
		}
		planArtifact, ok := parsePlanArtifact(raw)
		if !ok {
			candidateFailures = append(candidateFailures, fmt.Sprintf("candidate %d returned invalid plan JSON", i))
			continue
		}
		if forceFreshExternal {
			if planArtifact.Constraints == nil {
				planArtifact.Constraints = map[string]any{}
			}
			planArtifact.Constraints["needs_external_info"] = true
		}
		candidates = append(candidates, planArtifact)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("planner produced no valid candidates across %d pass(es): %s", planPasses, strings.Join(candidateFailures, "; "))
	}

	candidateJSON := make([]string, 0, len(candidates))
	for _, planArtifact := range candidates {
		candidateJSON = append(candidateJSON, mustPrettyJSON(planArtifact))
	}
	bestIdx, note := heuristicPlanSelection(candidateJSON, instruction, forceFreshExternal)
	if bestIdx < 0 || bestIdx >= len(candidates) {
		return fmt.Errorf("planner candidate selection returned invalid index %d for %d candidates", bestIdx, len(candidates))
	}
	planArtifact := candidates[bestIdx]
	planJSON := mustPrettyJSON(planArtifact)
	if err := r.writeArtifact(artifacts.KindPlan, planArtifact); err != nil {
		return err
	}
	if err := r.writeEvidence(evidence.Record{
		JobID:      r.claim.Job.ID,
		StepID:     r.claim.Step.ID,
		Kind:       evidence.KindModelJudgment,
		SourceType: "runtime_v2",
		SourceRef:  "plan",
		Summary:    "Planner emitted a typed plan artifact for downstream analysis and verification.",
		Confidence: 0.82,
		Metadata: map[string]any{
			"goal":            planArtifact.Goal,
			"subtask_count":   len(planArtifact.Subtasks),
			"constraint_keys": sortedMapKeys(planArtifact.Constraints),
		},
	}); err != nil {
		return err
	}
	r.svc.emitStepContext(r.claim.Step.ID, "plan_selection", fmt.Sprintf("best_index=%d note=%s", bestIdx+1, strings.TrimSpace(note)))
	r.svc.emitStepEvent(r.claim.Step.ID, "plan_complete", fmt.Sprintf("candidates=%d selected=%d", len(candidates), bestIdx+1))

	return r.complete("plan", planJSON, planJSON)
}

func (r *agentRuntime) runWebSearch() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "web_search_begin", "runtime=v2")
	mode := strings.ToLower(strings.TrimSpace(metadataString(r.claim.Job.Metadata, "web_search")))
	if mode == "" {
		mode = "auto"
	}
	if mode == "off" || mode == "false" || mode == "disabled" {
		output := "web search skipped: metadata mode=off"
		return r.complete("web_search", output, output)
	}
	if r.svc.webSearch == nil {
		output := "web search skipped: web search service disabled"
		return r.complete("web_search", output, output)
	}

	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	if isLocalClockOnlyInstruction(instruction) {
		localTime := strings.TrimSpace(metadataString(r.claim.Job.Metadata, "host_clock_local"))
		if localTime == "" {
			localTime = strings.TrimSpace(metadataString(r.claim.Job.Metadata, "client_time_local"))
		}
		output := "web search skipped: local clock question"
		if localTime != "" {
			output += " (" + localTime + ")"
		}
		return r.complete("web_search", output, output)
	}

	needsSearch := mode == "on" || mode == "force"
	if !needsSearch {
		planNeedsExternal, _ := planNeedsExternalInfo(r.contexts["plan"])
		needsSearch = planNeedsExternal || shouldForceFreshWebSearch(instruction, r.contexts["user_feedback"]) || isTimeSensitiveInstruction(instruction)
	}
	if !needsSearch {
		output := "web search skipped: heuristic not triggered"
		return r.complete("web_search", output, output)
	}

	query := strings.TrimSpace(metadataString(r.claim.Job.Metadata, "search_query"))
	if query == "" {
		query = instruction
	}
	if isTimeSensitiveInstruction(instruction) {
		query = anchorTimeSensitiveQuery(query, r.claim.Job)
	}
	query = sanitizeSearchQueryArtifacts(query)
	query = strings.TrimSpace(websearch.NormalizeQuery(query))
	if query == "" {
		output := "web search skipped: empty normalized query"
		return r.complete("web_search", output, output)
	}

	report, err := r.svc.webSearch.SearchAllDetailed(r.ctx, query)
	emitWebSearchProviderDiagnostics(r.svc, r.claim.Step.ID, report.Diagnostics)
	if err != nil {
		r.svc.emitStepEvent(r.claim.Step.ID, "web_search_failed", "reason=provider_failure")
		return fmt.Errorf("web search failed for query %q: %w", query, err)
	}
	results := report.Results
	webCtx := strings.TrimSpace(websearch.BuildContext(results, r.svc.contextBudget))
	if webCtx == "" {
		webCtx = "web search returned empty context"
	}
	if r.svc.v3Active() {
		documents := make([]artifacts.WebDocument, 0, len(results))
		for _, result := range results {
			documents = append(documents, artifacts.WebDocument{Provider: result.Provider, SearchURL: result.SearchURL, URL: result.URL, Title: result.Title, Snippet: result.Snippet, Content: trimForBudget(result.Content, 1200)})
			if err := r.writeEvidence(evidence.Record{
				JobID:      r.claim.Job.ID,
				StepID:     r.claim.Step.ID,
				Kind:       evidence.KindWebPage,
				SourceType: result.Provider,
				SourceRef:  result.URL,
				Excerpt:    trimForBudget(result.Content, 900),
				Summary:    firstNonEmpty(result.Title, result.Snippet, result.URL),
				Confidence: 0.78,
				Metadata:   map[string]any{"search_url": result.SearchURL},
			}); err != nil {
				return err
			}
		}
		if err := r.writeArtifact(artifacts.KindWebEvidence, artifacts.WebEvidenceArtifact{Query: query, Summary: webCtx, Documents: documents}); err != nil {
			return err
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "web_search_complete", fmt.Sprintf("query=%s", trimForBudget(query, 120)))
	return r.complete("web_search", webCtx, webCtx)
}

func (r *agentRuntime) runAnalyze() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "analyze_begin", "runtime=v2")
	modelName := r.svc.specialistModel(r.claim.Job, specialist.RoleAnalysisSpecialist, r.svc.pickThinkingModel(r.claim.Job, r.contexts, r.svc.models.Analyze))

	if isLowSignalChatInstruction(r.claim.Job.Instruction, r.claim.Job.Pipeline) {
		output := "Low-signal chat turn detected. Keep response concise and conversational."
		if r.svc.v3Active() {
			_ = r.writeArtifact(artifacts.KindAnalysis, artifacts.AnalysisArtifact{Summary: output})
		}
		return r.complete("analyzer", output, output)
	}

	prompt := strings.Join([]string{
		"You are an analysis specialist for an autonomous coding assistant.",
		"Return concise execution guidance, blockers, and assumptions.",
		promptBlock("User Instruction", r.claim.Job.Instruction),
		promptBlock("User Feedback", r.contexts["user_feedback"]),
		promptBlock("Plan", r.contexts["plan"]),
		promptBlock("Tooling", r.contexts["tooling"]),
		promptBlock("Workspace Context", r.contexts["workspace"]),
		promptBlock("Retrieved Memory", r.contexts["retrieval"]),
		promptBlock("Web Search Context", r.contexts["web_search"]),
		promptBlock("Recent Conversation", r.contexts["recent_conversation"]),
	}, "\n\n")

	analysis, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "analyze", modelName, prompt)
	if err != nil {
		return err
	}
	analysis = strings.TrimSpace(analysis)
	if analysis == "" {
		analysis = "No additional analysis generated."
	}

	if r.svc.v3Active() {
		if err := r.writeArtifact(artifacts.KindAnalysis, artifacts.AnalysisArtifact{Summary: analysis}); err != nil {
			return err
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "analyze_complete", fmt.Sprintf("chars=%d", len(analysis)))
	return r.complete("analyzer", analysis, analysis)
}

func (r *agentRuntime) runResponse() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "response_begin", "runtime=v2")

	roleID := specialist.RoleResponseSpecialist
	modelName := r.svc.pickThinkingModel(r.claim.Job, r.contexts, r.svc.models.Response)
	modelName = r.svc.specialistModel(r.claim.Job, roleID, modelName)

	if isFollowUpStatusCheckInstruction(r.claim.Job.Instruction, r.claim.Job.Pipeline) {
		status := "Still working"
		if strings.TrimSpace(r.contexts["verification"]) != "" {
			status = "Completed"
		}
		response := ensureResponseHasSources(status+".", r.claim.Job, r.contexts, nil)
		if r.svc.v3Active() {
			_ = r.writeArtifact(artifacts.KindResponseDraft, artifacts.ResponseDraftArtifact{Response: response})
		}
		return r.complete(r.action, response, response)
	}

	composerInstructions := r.svc.skillInstructions("response_composer")
	prompt := strings.Join([]string{
		"You are the final response specialist.",
		composerInstructions,
		"Use provided context only. Be direct and actionable.",
		"If web context is unavailable, say so plainly instead of fabricating facts.",
		promptBlock("User Instruction", r.claim.Job.Instruction),
		promptBlock("User Feedback", r.contexts["user_feedback"]),
		promptBlock("Plan", r.contexts["plan"]),
		promptBlock("Analysis", r.contexts["analyzer"]),
		promptBlock("Tooling", r.contexts["tooling"]),
		promptBlock("Workspace Context", r.contexts["workspace"]),
		promptBlock("Retrieved Memory", r.contexts["retrieval"]),
		promptBlock("Web Search Context", r.contexts["web_search"]),
		promptBlock("Recent Conversation", r.contexts["recent_conversation"]),
	}, "\n\n")

	response, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "response_draft", modelName, prompt)
	if err != nil {
		return err
	}
	response = strings.TrimSpace(response)
	if response == "" {
		response = strings.TrimSpace(r.contexts["analyzer"])
	}

	if shouldForceCodeOnlyResponse(r.claim.Job, r.contexts, modelName) {
		response = normalizeCodeOnlyResponse(response)
	}
	response = ensureResponseHasSources(response, r.claim.Job, r.contexts, nil)
	if strings.TrimSpace(response) == "" {
		response = "I could not generate a useful response from the current context."
	}
	if r.svc.v3Active() {
		if err := r.writeArtifact(artifacts.KindResponseDraft, artifacts.ResponseDraftArtifact{Response: response}); err != nil {
			return err
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "response_complete", fmt.Sprintf("chars=%d", len(response)))
	return r.complete(r.action, response, response)
}
