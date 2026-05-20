package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	runtimev3 "github.com/gryph/omnidex/internal/runtime/v3"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/verification"
	"github.com/gryph/omnidex/internal/websearch"
	"github.com/gryph/omnidex/internal/workspace"
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
		output := "noop: unsupported action " + r.action
		return r.complete(r.action, output, output)
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
	for i := 1; i <= planPasses; i++ {
		scope := fmt.Sprintf("plan_candidate_%d", i)
		prompt := r.planPrompt(forceFreshExternal, i, planPasses)
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, scope, modelName, prompt)
		if err != nil {
			r.svc.emitStepEvent(r.claim.Step.ID, "plan_candidate_error", fmt.Sprintf("index=%d error=%s", i, trimForBudget(err.Error(), 180)))
			continue
		}
		planArtifact, ok := parsePlanArtifact(raw)
		if !ok {
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
		fallback, ok := parsePlanArtifact(fallbackPlanCandidateForInstruction(instruction))
		if !ok {
			fallback = artifacts.PlanArtifact{
				Goal: instruction,
				Subtasks: []artifacts.Subtask{{
					ID:              "t1",
					Kind:            "respond",
					Objective:       "answer the user directly using available context",
					SuccessCriteria: []string{"response addresses the user request"},
				}},
			}
		}
		candidates = append(candidates, fallback)
	}

	candidateJSON := make([]string, 0, len(candidates))
	for _, planArtifact := range candidates {
		candidateJSON = append(candidateJSON, mustPrettyJSON(planArtifact))
	}
	bestIdx, note := heuristicPlanSelection(candidateJSON, instruction, forceFreshExternal)
	if bestIdx < 0 || bestIdx >= len(candidates) {
		bestIdx = 0
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

	results, err := r.svc.webSearch.SearchAll(r.ctx, query)
	if err != nil {
		output := "web search unavailable: " + trimForBudget(err.Error(), 240)
		return r.complete("web_search", output, output)
	}
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

func (r *agentRuntime) runVerify() error {
	r.svc.emitStepEvent(r.claim.Step.ID, "verify_begin", "runtime=v2")
	response := strings.TrimSpace(firstNonEmpty(
		r.contexts["assist"],
		r.contexts["roleplay"],
		r.contexts["narrate"],
	))
	if response == "" {
		response = strings.TrimSpace(r.claim.Job.Result)
	}

	if !reviewAlwaysEnabled(r.claim.Job) {
		summary := "verification skipped: review_always=off"
		if r.svc.v3Active() {
			_ = r.writeArtifact(artifacts.KindVerification, artifacts.VerificationArtifact{
				Verdict:            "skipped",
				RecommendedActions: []string{"verification disabled by job metadata"},
			})
		}
		if response == "" {
			return r.complete("verification", summary, summary)
		}
		return r.complete("verification", response, summary)
	}

	outcome := verificationOutcome{
		Status:     "pass",
		Confidence: 0.75,
		Summary:    "Response is present and ready.",
	}
	if isDeterministicLocalActionReviewInstruction(r.claim.Job.Instruction) {
		if deterministicOutcome, deterministicResponse, ok := evaluateDeterministicLocalActionReview(r.claim.Job.Instruction); ok {
			outcome = deterministicOutcome
			if strings.TrimSpace(deterministicResponse) != "" {
				response = strings.TrimSpace(deterministicResponse)
			}
		}
	}
	if strings.TrimSpace(response) == "" {
		outcome.Status = "retry"
		outcome.Confidence = 0.10
		outcome.Summary = "No response content was produced before verification."
		outcome.Gaps = append(outcome.Gaps, "missing response content")
	}
	if responseSeemsOffTopic(r.claim.Job.Instruction, response) {
		outcome.Status = "retry"
		outcome.Confidence = minFloat(outcome.Confidence, 0.25)
		outcome.Gaps = append(outcome.Gaps, "response appears off-topic for the current instruction")
	}

	audit := buildVerificationActionAudit(r.claim.Job, r.contexts)
	r.svc.emitStepContext(r.claim.Step.ID, "verification_action_audit", strings.TrimSpace(audit.Report))
	outcome, reviewSignals := enforceGroundingReview(outcome, r.claim.Job, response, r.contexts, testReport{})
	if len(reviewSignals) > 0 {
		r.svc.emitStepContext(r.claim.Step.ID, "verification_signals", strings.Join(reviewSignals, "\n"))
	}

	if outcome.Status != "pass" && persistentExecutionEnabled(r.claim.Job) && countAutoVerifyReplans(r.claim.Contexts) < maxAutoVerifyReplans {
		feedback, missingRequired, ok := autoVerifyReplanFeedback(r.claim.Job, r.contexts, r.claim.Contexts, outcome)
		if ok {
			if len(missingRequired) > 0 {
				r.svc.emitStepContext(r.claim.Step.ID, "verification_missing_required", strings.Join(missingRequired, ", "))
			}
			if _, err := r.svc.repo.ReplanJob(r.ctx, r.claim.Job.ID, feedback); err == nil {
				r.svc.emitStepEvent(r.claim.Step.ID, "verify_replan", "triggered=true")
				return nil
			}
		}
	}

	verificationSummary := strings.TrimSpace(strings.Join([]string{
		"status=" + strings.TrimSpace(outcome.Status),
		fmt.Sprintf("confidence=%.2f", outcome.Confidence),
		"summary=" + strings.TrimSpace(outcome.Summary),
	}, " "))
	if verificationSummary == "status= confidence=0.00 summary=" {
		verificationSummary = "status=retry confidence=0.10 summary=verification outcome was empty"
	}

	finalOutput := response
	if strings.TrimSpace(finalOutput) == "" {
		finalOutput = strings.TrimSpace(outcome.Summary)
	}
	if strings.TrimSpace(finalOutput) == "" {
		finalOutput = "Verification completed."
	}
	if outcome.Status != "pass" {
		finalOutput = strings.TrimSpace(strings.Join([]string{
			"INCOMPLETE: " + strings.TrimSpace(outcome.Summary),
			strings.TrimSpace(finalOutput),
		}, "\n\n"))
	}

	if r.svc.v3Active() {
		evidenceRecords, err := r.svc.repo.ListEvidenceByJob(r.ctx, r.claim.Job.ID)
		if err != nil {
			return err
		}
		assessments := verification.AssessClaims(finalOutput, evidenceRecords, 12)
		supportedClaims := make([]string, 0, len(assessments))
		unsupportedClaims := append([]string(nil), outcome.Gaps...)
		claimRecords := make([]model.ClaimRecord, 0, len(assessments))
		claimSupportIndex := make([][]int64, 0, len(assessments))
		claimSupportScores := make([]float64, 0, len(assessments))
		claimRationales := make([]string, 0, len(assessments))
		for _, assessment := range assessments {
			status := "unsupported"
			if assessment.Supported {
				status = "supported"
				supportedClaims = append(supportedClaims, assessment.Text)
			} else {
				unsupportedClaims = append(unsupportedClaims, assessment.Text)
			}
			claimRecords = append(claimRecords, model.ClaimRecord{JobID: r.claim.Job.ID, StepID: r.claim.Step.ID, Text: assessment.Text, NormalizedText: assessment.Normalized, Status: status, Confidence: assessment.SupportScore})
			claimSupportIndex = append(claimSupportIndex, append([]int64(nil), assessment.EvidenceRefs...))
			claimSupportScores = append(claimSupportScores, assessment.SupportScore)
			claimRationales = append(claimRationales, assessment.Rationale)
		}
		if len(assessments) > 0 && len(supportedClaims) == 0 {
			outcome.Status = "retry"
			outcome.Confidence = minFloat(outcome.Confidence, 0.35)
			unsupportedClaims = append(unsupportedClaims, "response claims are not explicitly supported by captured evidence")
		}
		savedClaims, err := r.svc.repo.WriteClaims(r.ctx, claimRecords)
		if err != nil {
			return err
		}
		supportLinks := make([]model.ClaimSupportRecord, 0, len(savedClaims)*2)
		for idx, claim := range savedClaims {
			for _, evidenceID := range claimSupportIndex[idx] {
				supportLinks = append(supportLinks, model.ClaimSupportRecord{ClaimID: claim.ID, EvidenceID: evidenceID, SupportScore: claimSupportScores[idx], Rationale: claimRationales[idx]})
			}
		}
		if err := r.svc.repo.WriteClaimSupports(r.ctx, supportLinks); err != nil {
			return err
		}

		verificationArtifact := artifacts.VerificationArtifact{
			Verdict:            outcome.Status,
			SupportedClaims:    supportedClaims,
			UnsupportedClaims:  unsupportedClaims,
			MissingEvidence:    append([]string(nil), audit.MissingRequired...),
			RecommendedActions: verifierRecommendedActions(outcome, audit.MissingRequired),
		}
		if err := r.writeArtifact(artifacts.KindVerification, verificationArtifact); err != nil {
			return err
		}
		if err := r.writeEvidence(evidence.Record{
			JobID:          r.claim.Job.ID,
			StepID:         r.claim.Step.ID,
			Kind:           evidence.KindModelJudgment,
			SourceType:     "runtime_v2",
			SourceRef:      "verification",
			Summary:        strings.TrimSpace(outcome.Summary),
			Confidence:     outcome.Confidence,
			SupportsClaims: verificationArtifact.SupportedClaims,
			Warnings:       append([]string(nil), verificationArtifact.UnsupportedClaims...),
			Metadata: map[string]any{
				"status":           outcome.Status,
				"missing_required": audit.MissingRequired,
				"review_signals":   reviewSignals,
			},
		}); err != nil {
			return err
		}
	}

	r.svc.emitStepEvent(r.claim.Step.ID, "verify_complete", verificationSummary)
	return r.complete("verification", finalOutput, verificationSummary)
}

func (r *agentRuntime) planPrompt(forceFreshExternal bool, passIndex int, passTotal int) string {
	mode := "normal"
	if forceFreshExternal {
		mode = "fresh_external_required"
	}
	plannerInstructions := r.svc.skillInstructions("executive_planner")
	return strings.Join([]string{
		"You are the planner specialist.",
		plannerInstructions,
		`Return JSON only with schema: {"goal":"...","constraints":{"needs_external_info":bool,"required_tools":["..."],"mode":"..."},"subtasks":[{"id":"t1","kind":"research|analyze|respond|verify","objective":"...","inputs":["..."],"outputs":["..."],"success_criteria":["..."]}]}`,
		fmt.Sprintf("Planning pass: %d/%d", passIndex, passTotal),
		"Planning mode: " + mode,
		"If tools are unavailable, include alternatives in constraints and shape subtasks around what is available.",
		"If external freshness is required, set constraints.needs_external_info=true.",
		promptBlock("User Instruction", r.claim.Job.Instruction),
		promptBlock("User Feedback", r.contexts["user_feedback"]),
		promptBlock("Tooling", r.contexts["tooling"]),
		promptBlock("Workspace Context", r.contexts["workspace"]),
		promptBlock("Retrieved Memory", r.contexts["retrieval"]),
		promptBlock("Web Search Context", r.contexts["web_search"]),
		promptBlock("Planner Action Catalog", plannerActionCatalog(r.claim.Job)),
	}, "\n\n")
}

func (r *agentRuntime) ensureV3IntentArtifact() error {
	if !r.svc.v3Active() || r.svc.v3Engine == nil {
		return nil
	}
	if _, ok, err := r.svc.repo.LatestArtifact(r.ctx, r.claim.Job.ID, artifacts.KindIntent); err != nil {
		return err
	} else if ok {
		return nil
	}
	return r.svc.v3Engine.Bootstrap(r.ctx, runtimev3.RunInput{
		JobID:       r.claim.Job.ID,
		StepID:      r.claim.Step.ID,
		Instruction: r.claim.Job.Instruction,
		Pipeline:    r.claim.Job.Pipeline,
	})
}

func (r *agentRuntime) writeArtifact(kind string, payload any) error {
	if !r.svc.v3Active() {
		return nil
	}
	env, err := artifacts.MarshalPayload(kind, "1", payload)
	if err != nil {
		return err
	}
	env.JobID = r.claim.Job.ID
	env.StepID = r.claim.Step.ID
	return r.svc.repo.WriteArtifact(r.ctx, env)
}

func (r *agentRuntime) latestArtifact(kind string) (artifacts.Envelope, bool, error) {
	if !r.svc.v3Active() {
		return artifacts.Envelope{}, false, nil
	}
	return r.svc.repo.LatestArtifact(r.ctx, r.claim.Job.ID, kind)
}

func (r *agentRuntime) writeEvidence(record evidence.Record) error {
	if !r.svc.v3Active() {
		return nil
	}
	if record.JobID == 0 {
		record.JobID = r.claim.Job.ID
	}
	if record.StepID == 0 {
		record.StepID = r.claim.Step.ID
	}
	return r.svc.repo.WriteEvidence(r.ctx, record)
}

func (r *agentRuntime) inferRequiredTools() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	appendTool := func(tool string) {
		tool = strings.ToLower(strings.TrimSpace(tool))
		if tool == "" {
			return
		}
		if _, ok := seen[tool]; ok {
			return
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}
	for _, tool := range r.requiredToolsFromPlan() {
		appendTool(tool)
	}
	instruction := strings.ToLower(strings.TrimSpace(r.claim.Job.Instruction + "\n" + r.contexts["user_feedback"]))
	if instruction != "" {
		if containsAnyToken(instruction, "code", "repo", "repository", "file", "golang", "go", "postgres", "project", "workspace") {
			appendTool("workspace")
		}
		if containsAnyToken(instruction, "latest", "recent", "today", "current", "news", "price", "weather", "who is", "what's happening") {
			appendTool("web_search")
		}
		if containsAnyToken(instruction, "run", "test", "compile", "build", "execute", "command", "shell") {
			appendTool("shell_exec")
		}
		if containsAnyToken(instruction, "memory", "remember", "history", "previous", "earlier") {
			appendTool("memory")
		}
	}
	sort.Strings(out)
	return out
}

func parsePlanArtifact(raw string) (artifacts.PlanArtifact, bool) {
	raw = bestEffortJSONObject(raw)
	if raw == "" {
		return artifacts.PlanArtifact{}, false
	}
	var direct artifacts.PlanArtifact
	if json.Unmarshal([]byte(raw), &direct) == nil && strings.TrimSpace(direct.Goal) != "" {
		direct = normalizePlanArtifact(direct)
		return direct, true
	}
	payload, ok := parsePlanPayload(raw)
	if !ok {
		return artifacts.PlanArtifact{}, false
	}
	plan := artifacts.PlanArtifact{
		Goal:        strings.TrimSpace(fmt.Sprintf("%v", payload["goal"])),
		Constraints: map[string]any{},
	}
	if plan.Goal == "" {
		plan.Goal = "produce a grounded answer"
	}
	if needsExternal, ok := payload["needs_external_info"].(bool); ok {
		plan.Constraints["needs_external_info"] = needsExternal
	}
	if required := parseAnyStringSlice(payload["required_tools"]); len(required) > 0 {
		plan.Constraints["required_tools"] = required
	}
	if clarifications := parseAnyStringSlice(payload["clarifications"]); len(clarifications) > 0 {
		plan.Constraints["clarifications"] = clarifications
	}
	for idx, task := range parseAnyStringSlice(payload["tasks"]) {
		plan.Subtasks = append(plan.Subtasks, artifacts.Subtask{
			ID:              fmt.Sprintf("t%d", idx+1),
			Kind:            guessSubtaskKind(task),
			Objective:       task,
			Inputs:          []string{"instruction", "available_context"},
			Outputs:         []string{"grounded_progress"},
			SuccessCriteria: parseAnyStringSlice(payload["done_when"]),
		})
	}
	plan = normalizePlanArtifact(plan)
	return plan, true
}

func normalizePlanArtifact(plan artifacts.PlanArtifact) artifacts.PlanArtifact {
	plan.Goal = strings.TrimSpace(plan.Goal)
	if plan.Goal == "" {
		plan.Goal = "produce a grounded answer"
	}
	if plan.Constraints == nil {
		plan.Constraints = map[string]any{}
	}
	normalized := make([]artifacts.Subtask, 0, len(plan.Subtasks))
	for idx, sub := range plan.Subtasks {
		if strings.TrimSpace(sub.ID) == "" {
			sub.ID = fmt.Sprintf("t%d", idx+1)
		}
		if strings.TrimSpace(sub.Kind) == "" {
			sub.Kind = guessSubtaskKind(sub.Objective)
		}
		sub.Objective = strings.TrimSpace(sub.Objective)
		if sub.Objective == "" {
			continue
		}
		if len(sub.SuccessCriteria) == 0 {
			sub.SuccessCriteria = []string{"subtask output is usable downstream"}
		}
		normalized = append(normalized, sub)
	}
	if len(normalized) == 0 {
		normalized = []artifacts.Subtask{{
			ID:              "t1",
			Kind:            "respond",
			Objective:       "answer the user directly using available context",
			SuccessCriteria: []string{"response addresses the user request"},
		}}
	}
	plan.Subtasks = normalized
	return plan
}

func parseAnyStringSlice(value any) []string {
	items, ok := value.([]any)
	if ok {
		out := make([]string, 0, len(items))
		for _, item := range items {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	}
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" {
		return nil
	}
	return []string{text}
}

func guessSubtaskKind(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case containsAnyToken(lower, "search", "research", "look up", "latest"):
		return "research"
	case containsAnyToken(lower, "analyze", "compare", "reason", "review"):
		return "analyze"
	case containsAnyToken(lower, "verify", "check", "validate", "confirm"):
		return "verify"
	default:
		return "respond"
	}
}

func containsAnyToken(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func mustPrettyJSON(value any) string {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func verifierRecommendedActions(outcome verificationOutcome, missingRequired []string) []string {
	actions := make([]string, 0, len(missingRequired)+2)
	for _, item := range missingRequired {
		item = strings.TrimSpace(item)
		if item != "" {
			actions = append(actions, "provide or verify required step: "+item)
		}
	}
	if outcome.Status != "pass" && strings.TrimSpace(outcome.Summary) != "" {
		actions = append(actions, "repair response: "+strings.TrimSpace(outcome.Summary))
	}
	if len(actions) == 0 {
		actions = append(actions, "no additional work required")
	}
	return actions
}

func sortedMapKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapWorkspaceExcerpts(items []workspace.FileExcerpt) []artifacts.WorkspaceFileExcerpt {
	if len(items) == 0 {
		return nil
	}
	out := make([]artifacts.WorkspaceFileExcerpt, 0, len(items))
	for _, item := range items {
		out = append(out, artifacts.WorkspaceFileExcerpt{Path: item.Path, Reason: item.Reason, Excerpt: item.Excerpt, Score: item.Score, Language: item.Language, Symbols: append([]string(nil), item.Symbols...)})
	}
	return out
}

func (r *agentRuntime) complete(contextKey string, output string, contextValue string) error {
	output = strings.TrimSpace(output)
	contextValue = strings.TrimSpace(contextValue)
	if contextValue == "" {
		contextValue = output
	}
	r.contexts[contextKey] = contextValue
	return r.svc.repo.CompleteStep(r.ctx, r.claim.Step.ID, output, contextKey, contextValue)
}

func (r *agentRuntime) requiredToolsFromPlan() []string {
	payload, ok := parsePlanPayload(r.contexts["plan"])
	if !ok {
		return nil
	}
	raw, ok := payload["required_tools"]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		tool := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", item)))
		if tool == "" {
			continue
		}
		if _, exists := seen[tool]; exists {
			continue
		}
		seen[tool] = struct{}{}
		out = append(out, tool)
	}
	sort.Strings(out)
	return out
}

func (r *agentRuntime) missingTools(required []string, hostTools []string) []string {
	if len(required) == 0 {
		return nil
	}
	hostSet := make(map[string]struct{}, len(hostTools))
	for _, tool := range hostTools {
		hostSet[strings.ToLower(strings.TrimSpace(tool))] = struct{}{}
	}
	missing := make([]string, 0, len(required))
	for _, tool := range required {
		if !hostToolAvailable(tool, hostSet) {
			missing = append(missing, tool)
		}
	}
	sort.Strings(missing)
	return missing
}

func bestEffortJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		return raw
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(raw[start : end+1])
	}
	return raw
}

func splitCSVTags(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, raw := range parts {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func llmFallbackTags(instruction string, max int) []string {
	if max <= 0 {
		max = 8
	}
	tokens := tokenWordPattern.FindAllString(strings.ToLower(instruction), -1)
	if len(tokens) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(max, len(tokens)))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if len(token) < 3 {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
		if len(out) >= max {
			break
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func csvOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
