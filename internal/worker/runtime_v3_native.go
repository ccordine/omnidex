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
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/specialist"
	toolruntime "github.com/gryph/omnidex/internal/tools"
	"github.com/gryph/omnidex/internal/verification"
	"github.com/gryph/omnidex/internal/websearch"
)

const maxDelegatedSubtasks = 6

func missingTools(required []string, hostTools []string) []string {
	if len(required) == 0 {
		return nil
	}
	hostSet := make(map[string]struct{}, len(hostTools))
	for _, tool := range hostTools {
		hostSet[strings.ToLower(strings.TrimSpace(tool))] = struct{}{}
	}
	out := make([]string, 0, len(required))
	for _, tool := range required {
		clean := strings.ToLower(strings.TrimSpace(tool))
		if clean == "" {
			continue
		}
		if _, ok := hostSet[clean]; ok {
			continue
		}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

type nativeRuntimeV3 struct {
	svc      *Service
	ctx      context.Context
	claim    *model.ClaimedStep
	action   string
	contexts map[string]string
}

func (s *Service) runNativeV3Step(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	rt := &nativeRuntimeV3{svc: s, ctx: ctx, claim: claim, action: strings.ToLower(strings.TrimSpace(action)), contexts: contexts}
	return rt.run()
}

func (r *nativeRuntimeV3) run() error {
	switch r.action {
	case "v3_intent_parse":
		return r.runIntentParse()
	case "v3_capability_audit":
		return r.runCapabilityAudit()
	case "v3_workspace_research":
		return r.runWorkspaceResearch()
	case "v3_memory_retrieval":
		return r.runMemoryRetrieval()
	case "v3_planning":
		return r.runPlanning()
	case "v3_external_research":
		return r.runExternalResearch()
	case "v3_subtask":
		return r.runSubtask()
	case "v3_analysis":
		return r.runAnalysis()
	case "v3_response_draft":
		return r.runResponseDraft()
	case "v3_verification":
		return r.runVerification()
	case "v3_memory_review":
		return r.runMemoryReview()
	case "v3_finalize":
		return r.runFinalize()
	default:
		return r.complete(r.action, "unsupported native v3 action: "+r.action, "unsupported native v3 action: "+r.action)
	}
}

func (r *nativeRuntimeV3) runIntentParse() error {
	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	feedback := strings.TrimSpace(r.contexts["user_feedback"])
	intent := artifacts.IntentArtifact{
		UserGoal:       instruction,
		Mode:           r.claim.Job.Pipeline,
		RequiresAction: requiresActionHeuristically(instruction),
		Ambiguities:    collectAmbiguities(instruction),
		Constraints:    collectConstraints(instruction, feedback),
	}
	if err := r.writeArtifact(artifacts.KindIntent, intent); err != nil {
		return err
	}
	summary := strings.Join([]string{
		"goal=" + safeLine(trimForBudget(intent.UserGoal, 160), "none"),
		fmt.Sprintf("requires_action=%t", intent.RequiresAction),
		"constraints=" + csvOrNone(intent.Constraints),
		"ambiguities=" + csvOrNone(intent.Ambiguities),
	}, "\n")
	return r.complete("intent", summary, summary)
}

func (r *nativeRuntimeV3) runCapabilityAudit() error {
	hostTools := metadataCSV(r.claim.Job.Metadata, "host_tools_available")
	packageManagers := resolvePackageManagers(r.claim.Job)
	requiredTools := r.inferRequiredTools()
	missingTools := missingTools(requiredTools, hostTools)
	allowedTools := make([]string, 0, len(requiredTools))
	missingSet := map[string]struct{}{}
	for _, tool := range missingTools {
		missingSet[tool] = struct{}{}
	}
	for _, tool := range requiredTools {
		if _, ok := missingSet[tool]; ok {
			continue
		}
		allowedTools = append(allowedTools, tool)
	}
	availableTools := r.availableToolNames("capability_auditor")
	if result, err := r.svc.executeV3Tool(r.ctx, r.claim, "capability_auditor", toolruntime.Call{
		Name: "tool.registry",
	}); err == nil {
		catalog, decodeErr := decodeToolOutput[struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}](result)
		if decodeErr == nil && len(catalog.Tools) > 0 {
			availableTools = availableTools[:0]
			for _, item := range catalog.Tools {
				if strings.TrimSpace(item.Name) != "" {
					availableTools = append(availableTools, strings.TrimSpace(item.Name))
				}
			}
		}
	}
	audit := artifacts.CapabilityAuditArtifact{
		AllowedTools:   allowedTools,
		AvailableTools: availableTools,
		MissingTools:   missingTools,
		WorkspaceOK:    r.svc.workspace != nil && r.svc.workspace.Enabled(),
		WebSearchOK:    r.svc.webSearch != nil,
		Notes:          []string{"native-v3 capability audit", "tool availability inferred before planning", "available tool definitions resolved from the file-backed specialist registry"},
	}
	if err := r.writeArtifact(artifacts.KindCapabilityAudit, audit); err != nil {
		return err
	}
	if err := r.writeEvidence(evidence.Record{Kind: evidence.KindModelJudgment, SourceType: "runtime_v3", SourceRef: "capability_audit", Summary: "Capability audit completed.", Confidence: 0.92, Metadata: map[string]any{"required_tools": requiredTools, "missing_tools": missingTools, "package_managers": packageManagers}}); err != nil {
		return err
	}
	output := strings.Join([]string{
		"required_tools=" + csvOrNone(requiredTools),
		"allowed_tools=" + csvOrNone(allowedTools),
		"available_tools=" + csvOrNone(availableTools),
		"missing_tools=" + csvOrNone(missingTools),
		"package_managers=" + csvOrNone(packageManagers),
	}, "\n")
	return r.complete("capability_audit", output, output)
}

func (r *nativeRuntimeV3) runWorkspaceResearch() error {
	if r.svc.workspace == nil || !r.svc.workspace.Enabled() {
		return r.complete("workspace", "workspace research skipped: service disabled", "workspace research skipped: service disabled")
	}
	query := strings.TrimSpace(strings.Join([]string{r.claim.Job.Instruction, r.contexts["user_feedback"], r.collectDelegatedObjectives()}, "\n"))
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "workspace_researcher", toolruntime.Call{
		Name: "workspace.research",
		Input: map[string]any{
			"query": query,
		},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.WorkspaceArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindWorkspace, artifact); err != nil {
		return err
	}
	return r.complete("workspace", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runMemoryRetrieval() error {
	instruction := strings.TrimSpace(r.claim.Job.Instruction)
	feedback := strings.TrimSpace(r.contexts["user_feedback"])
	retrieveHistorical, reason := shouldRetrieveHistoricalMemory(r.claim.Job, r.contexts)
	if !retrieveHistorical {
		output := strings.TrimSpace("Historical memory retrieval skipped: " + reason)
		if output == "Historical memory retrieval skipped:" {
			output = "Historical memory retrieval skipped: retrieval not required for this turn."
		}
		artifact := artifacts.RetrievalArtifact{Summary: output}
		if err := r.writeArtifact(artifacts.KindRetrieval, artifact); err != nil {
			return err
		}
		return r.complete("retrieval", output, output)
	}
	limit := resolveMemoryRetrievalLimit(r.claim.Job, instruction, feedback, r.svc.retrievalLimit)
	content := strings.TrimSpace(strings.Join([]string{instruction, feedback, r.collectDelegatedObjectives()}, "\n"))
	scopeTags := memoryScopeTags(r.claim.Job, splitCSVTags(r.contexts["tags"]))
	sessionID := metadataString(r.claim.Job.Metadata, "session_id")
	sessionTag := ""
	if sessionID != "" {
		sessionTag = "session:" + sessionID
	}
	projectScope := projectTag(r.claim.Job)
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "memory_retriever", toolruntime.Call{
		Name: "memory.retrieve",
		Input: map[string]any{
			"query":       content,
			"limit":       limit,
			"scope_tags":  scopeTags,
			"project_tag": projectScope,
			"session_tag": sessionTag,
		},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.RetrievalArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindRetrieval, artifact); err != nil {
		return err
	}
	return r.complete("retrieval", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runPlanning() error {
	capability, _ := r.readCapabilityAudit()
	workspaceArtifact, _ := r.readWorkspaceArtifact()
	retrievalArtifact, _ := r.readRetrievalArtifact()
	modelName := r.svc.skillPreferredModel("executive_planner", r.svc.specialistModel(r.claim.Job, specialist.RolePlannerSpecialist, r.svc.models.Plan))
	prompt := strings.Join([]string{
		"You are the executive planner for Omnidex v3.",
		r.svc.skillInstructions("executive_planner"),
		`Return JSON only with schema: {"goal":"...","constraints":{"needs_external_info":bool,"required_tools":["..."],"mode":"..."},"subtasks":[{"id":"t1","kind":"research|analyze|respond|verify|memory_review","objective":"...","inputs":["..."],"outputs":["..."],"success_criteria":["..."]}]}`,
		"Create bounded subtasks. Use research subtasks for targeted information gathering. Do not create duplicate subtasks.",
		promptBlock("Instruction", r.claim.Job.Instruction),
		promptBlock("User Feedback", r.contexts["user_feedback"]),
		promptBlock("Capability Audit", summarizeCapabilityAudit(capability)),
		promptBlock("Workspace", workspaceArtifact.Summary),
		promptBlock("Retrieved Memory", retrievalArtifact.Summary),
	}, "\n\n")
	raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "v3_planning", modelName, prompt)
	if err != nil {
		return err
	}
	plan, ok := parsePlanArtifact(raw)
	if !ok {
		plan = artifacts.PlanArtifact{Goal: "answer the user accurately", Constraints: map[string]any{"needs_external_info": shouldUseWebSearch(r.claim.Job.Instruction, r.contexts)}, Subtasks: []artifacts.Subtask{{ID: "t1", Kind: "research", Objective: "gather the strongest supporting context", Inputs: []string{"workspace", "retrieval"}, Outputs: []string{"grounded_findings"}, SuccessCriteria: []string{"evidence is captured"}}}}
		plan = normalizePlanArtifact(plan)
	}
	if capability.MissingTools != nil && len(capability.MissingTools) > 0 {
		plan.Constraints["missing_tools"] = capability.MissingTools
	}
	if len(workspaceArtifact.Languages) > 0 {
		plan.Constraints["workspace_languages"] = workspaceArtifact.Languages
	}
	if err := r.writeArtifact(artifacts.KindPlan, plan); err != nil {
		return err
	}
	count, err := r.svc.repo.CountStepsByAction(r.ctx, r.claim.Job.ID, "v3_subtask")
	if err != nil {
		return err
	}
	if count == 0 {
		filtered := filterDelegatedSubtasks(plan.Subtasks)
		if len(filtered) > 0 {
			if _, err := r.svc.repo.ExpandDelegatedSubtasks(r.ctx, r.claim.Job.ID, r.claim.Step.ID, filtered); err != nil {
				return err
			}
		}
	}
	planJSON, _ := json.Marshal(plan)
	return r.complete("plan", string(planJSON), string(planJSON))
}

func (r *nativeRuntimeV3) runExternalResearch() error {
	plan, _ := r.readPlanArtifact()
	query := r.claim.Job.Instruction
	if len(plan.Subtasks) > 0 {
		query = query + "\n" + joinSubtaskObjectives(plan.Subtasks)
	}
	if !needsExternalResearch(plan, r.claim.Job.Instruction, r.contexts) || r.svc.webSearch == nil {
		artifact := artifacts.WebEvidenceArtifact{Query: query, Summary: "external research skipped"}
		if err := r.writeArtifact(artifacts.KindWebEvidence, artifact); err != nil {
			return err
		}
		return r.complete("web_search", artifact.Summary, artifact.Summary)
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "web_researcher", toolruntime.Call{
		Name: "web.search",
		Input: map[string]any{
			"query": query,
		},
	})
	if err != nil {
		return err
	}
	artifact, err := decodeToolOutput[artifacts.WebEvidenceArtifact](result)
	if err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindWebEvidence, artifact); err != nil {
		return err
	}
	return r.complete("web_search", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runSubtask() error {
	summary, sources, err := r.runSubtaskWithTools()
	if err == nil && strings.TrimSpace(summary) != "" {
		subtaskID := strings.TrimSpace(r.contexts["subtask_id"])
		kind := strings.TrimSpace(r.contexts["subtask_kind"])
		objective := strings.TrimSpace(r.contexts["subtask_objective"])
		if objective == "" {
			objective = "execute delegated subtask"
		}
		artifact := artifacts.SubtaskResultArtifact{SubtaskID: subtaskID, Kind: kind, Objective: objective, Summary: strings.TrimSpace(summary), Sources: sources}
		if err := r.writeArtifact(artifacts.KindSubtaskResult, artifact); err != nil {
			return err
		}
		key := "subtask:" + safeLine(subtaskID, fmt.Sprintf("step-%d", r.claim.Step.ID))
		return r.complete(key, artifact.Summary, artifact.Summary)
	}
	return r.runSubtaskLegacy()
}

func (r *nativeRuntimeV3) runSubtaskLegacy() error {
	subtaskID := strings.TrimSpace(r.contexts["subtask_id"])
	kind := strings.TrimSpace(r.contexts["subtask_kind"])
	objective := strings.TrimSpace(r.contexts["subtask_objective"])
	if objective == "" {
		objective = "execute delegated subtask"
	}
	sections := []string{"Subtask objective: " + objective}
	sources := make([]string, 0, 4)
	if r.svc.workspace != nil && r.svc.workspace.Enabled() {
		research, err := r.svc.workspace.Research(objective)
		if err == nil && strings.TrimSpace(research.Summary) != "" {
			sections = append(sections, "Workspace:\n"+trimForBudget(research.Summary, 1600))
			sources = append(sources, "workspace")
			for _, excerpt := range research.Excerpts[:minInt(len(research.Excerpts), 3)] {
				if err := r.writeEvidence(evidence.Record{Kind: evidence.KindFileExcerpt, SourceType: "workspace", SourceRef: excerpt.Path, FilePaths: []string{excerpt.Path}, Excerpt: excerpt.Excerpt, Summary: excerpt.Reason, Confidence: excerpt.Score, Metadata: map[string]any{"language": excerpt.Language, "symbols": excerpt.Symbols, "subtask_id": subtaskID}}); err != nil {
					return err
				}
			}
		}
	}
	if r.svc.webSearch != nil && (strings.EqualFold(kind, "research") || strings.EqualFold(kind, "web_research") || shouldUseWebSearch(objective)) {
		results, err := r.svc.webSearch.SearchAll(r.ctx, objective)
		if err == nil && len(results) > 0 {
			sections = append(sections, "Web:\n"+trimForBudget(websearch.BuildContext(results, r.svc.contextBudget), 1600))
			sources = append(sources, "web")
			for _, result := range results[:minInt(len(results), 2)] {
				if err := r.writeEvidence(evidence.Record{Kind: evidence.KindWebPage, SourceType: result.Provider, SourceRef: result.URL, Summary: safeLine(result.Title, result.URL), Excerpt: trimForBudget(result.Content, 1000), Confidence: 0.78, Metadata: map[string]any{"subtask_id": subtaskID}}); err != nil {
					return err
				}
			}
		}
	}
	prompt := strings.Join([]string{
		"You are a delegated subtask worker for Omnidex v3.",
		"Return a concise grounded summary of the subtask outcome.",
		promptBlock("Subtask ID", subtaskID),
		promptBlock("Subtask Kind", kind),
		promptBlock("Objective", objective),
		promptBlock("Collected Context", strings.Join(sections, "\n\n")),
	}, "\n\n")
	modelName := r.svc.skillPreferredModel("subtask_executor", r.svc.specialistModel(r.claim.Job, specialist.RoleAnalysisSpecialist, r.svc.models.Fast))
	summary, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "v3_subtask", modelName, prompt)
	if err != nil {
		summary = strings.Join(sections, "\n\n")
	}
	artifact := artifacts.SubtaskResultArtifact{SubtaskID: subtaskID, Kind: kind, Objective: objective, Summary: strings.TrimSpace(summary), Sources: sources}
	if err := r.writeArtifact(artifacts.KindSubtaskResult, artifact); err != nil {
		return err
	}
	key := "subtask:" + safeLine(subtaskID, fmt.Sprintf("step-%d", r.claim.Step.ID))
	return r.complete(key, artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runAnalysis() error {
	plan, _ := r.readPlanArtifact()
	workspaceArtifact, _ := r.readWorkspaceArtifact()
	retrievalArtifact, _ := r.readRetrievalArtifact()
	webArtifact, _ := r.readWebArtifact()
	delegated := r.collectSubtaskOutputs()
	prompt := strings.Join([]string{
		"You are the analysis specialist for Omnidex v3.",
		"Summarize the highest-confidence findings, blockers, and assumptions. Be specific and grounded.",
		promptBlock("Instruction", r.claim.Job.Instruction),
		promptBlock("Plan", planSummary(plan)),
		promptBlock("Workspace", workspaceArtifact.Summary),
		promptBlock("Memory", retrievalArtifact.Summary),
		promptBlock("Web", webArtifact.Summary),
		promptBlock("Delegated Subtasks", strings.Join(delegated, "\n\n")),
	}, "\n\n")
	modelName := r.svc.skillPreferredModel("analysis_specialist", r.svc.specialistModel(r.claim.Job, specialist.RoleAnalysisSpecialist, r.svc.models.Analyze))
	summary, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "v3_analysis", modelName, prompt)
	if err != nil {
		return err
	}
	artifact := artifacts.AnalysisArtifact{Summary: strings.TrimSpace(summary), DelegatedSubtasks: delegated, Blockers: extractBlockingBullets(summary), Assumptions: extractAssumptions(summary)}
	if err := r.writeArtifact(artifacts.KindAnalysis, artifact); err != nil {
		return err
	}
	return r.complete("analysis", artifact.Summary, artifact.Summary)
}

func (r *nativeRuntimeV3) runResponseDraft() error {
	analysisArtifact, _ := r.readAnalysisArtifact()
	webArtifact, _ := r.readWebArtifact()
	if autonomyEnabled(r.claim.Job) && isSimpleFileTaskInstruction(r.claim.Job.Instruction, r.claim.Job.Pipeline) {
		artifact := artifacts.ResponseDraftArtifact{Response: strings.TrimSpace(simpleFileTaskFallbackResponse(r.claim.Job))}
		if err := r.writeArtifact(artifacts.KindResponseDraft, artifact); err != nil {
			return err
		}
		if err := r.svc.inferMemory(r.ctx, r.claim.Step.ID, r.claim.Job, r.contexts, artifact.Response); err != nil {
			return err
		}
		return r.complete("response_draft", artifact.Response, artifact.Response)
	}
	responseInstructions := r.svc.skillInstructions("response_composer")
	prompt := strings.Join([]string{
		"You are the response composer for Omnidex v3.",
		responseInstructions,
		"Write the best grounded response possible with the available evidence. Avoid pretending unsupported actions happened.",
		promptBlock("Instruction", r.claim.Job.Instruction),
		promptBlock("Analysis", analysisArtifact.Summary),
		promptBlock("Web Evidence", webArtifact.Summary),
		promptBlock("Workspace and Memory", strings.Join([]string{r.contexts["workspace"], r.contexts["retrieval"]}, "\n\n")),
	}, "\n\n")
	modelName := r.svc.skillPreferredModel("response_composer", r.svc.specialistModel(r.claim.Job, specialist.RoleResponseSpecialist, r.svc.models.Response))
	draft, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "v3_response_draft", modelName, prompt)
	if err != nil {
		return err
	}
	artifact := artifacts.ResponseDraftArtifact{Response: strings.TrimSpace(draft)}
	if err := r.writeArtifact(artifacts.KindResponseDraft, artifact); err != nil {
		return err
	}
	if err := r.svc.inferMemory(r.ctx, r.claim.Step.ID, r.claim.Job, r.contexts, artifact.Response); err != nil {
		return err
	}
	return r.complete("response_draft", artifact.Response, artifact.Response)
}

func (r *nativeRuntimeV3) runVerification() error {
	draft, _ := r.readResponseDraftArtifact()
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "verifier", toolruntime.Call{
		Name: "evidence.inspect",
		Input: map[string]any{
			"job_id": r.claim.Job.ID,
		},
	})
	if err != nil {
		return err
	}
	toolOutput, err := decodeToolOutput[struct {
		Summary string           `json:"summary"`
		Records []map[string]any `json:"records"`
	}](result)
	if err != nil {
		return err
	}
	evidenceRecords := evidenceRecordsFromMaps(toolOutput.Records)
	assessments := verification.AssessClaims(draft.Response, evidenceRecords, 16)
	supportedClaims := make([]string, 0, len(assessments))
	unsupportedClaims := make([]string, 0, len(assessments))
	claimRecords := make([]model.ClaimRecord, 0, len(assessments))
	supportLinks := make([]model.ClaimSupportRecord, 0, len(assessments)*2)
	for _, assessment := range assessments {
		status := "unsupported"
		if assessment.Supported {
			status = "supported"
			supportedClaims = append(supportedClaims, assessment.Text)
		} else {
			unsupportedClaims = append(unsupportedClaims, assessment.Text)
		}
		claimRecords = append(claimRecords, model.ClaimRecord{JobID: r.claim.Job.ID, StepID: r.claim.Step.ID, Text: assessment.Text, NormalizedText: assessment.Normalized, Status: status, Confidence: assessment.SupportScore})
	}
	savedClaims, err := r.svc.repo.WriteClaims(r.ctx, claimRecords)
	if err != nil {
		return err
	}
	for idx, claim := range savedClaims {
		for _, assessment := range assessments[idx].EvidenceRefs {
			supportLinks = append(supportLinks, model.ClaimSupportRecord{ClaimID: claim.ID, EvidenceID: assessment, SupportScore: assessments[idx].SupportScore, Rationale: assessments[idx].Rationale})
		}
	}
	if err := r.svc.repo.WriteClaimSupports(r.ctx, supportLinks); err != nil {
		return err
	}
	verdict := "pass"
	recommended := []string{"proceed to finalize"}
	if len(unsupportedClaims) > 0 {
		verdict = "retry"
		recommended = []string{"trim or qualify unsupported claims before finalizing"}
	}
	artifact := artifacts.VerificationArtifact{Verdict: verdict, SupportedClaims: supportedClaims, UnsupportedClaims: unsupportedClaims, RecommendedActions: recommended}
	if err := r.writeArtifact(artifacts.KindVerification, artifact); err != nil {
		return err
	}
	if err := r.writeEvidence(evidence.Record{Kind: evidence.KindModelJudgment, SourceType: "runtime_v3", SourceRef: "verification", Summary: "Verification complete.", Confidence: 0.84, SupportsClaims: supportedClaims, Warnings: unsupportedClaims}); err != nil {
		return err
	}
	summary := strings.Join([]string{"verdict=" + verdict, fmt.Sprintf("supported_claims=%d", len(supportedClaims)), fmt.Sprintf("unsupported_claims=%d", len(unsupportedClaims))}, "\n")
	return r.complete("verification", summary, summary)
}

func (r *nativeRuntimeV3) runMemoryReview() error {
	candidates, err := r.svc.repo.ListMemoryCandidates(r.ctx, r.claim.Job.ID, "candidate", 24)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return r.complete("memory_review", "memory review: no pending candidates", "memory review: no pending candidates")
	}
	promoted := make([]string, 0, len(candidates))
	rejected := make([]string, 0, len(candidates))
	tags := memoryScopeTags(r.claim.Job, splitCSVTags(r.contexts["tags"]))
	for _, candidate := range candidates {
		decision := reviewMemoryCandidate(candidate, r.claim.Job)
		if decision == model.MemoryCandidateStatusRejected {
			rejected = append(rejected, candidate.Content)
			_ = r.svc.repo.UpdateMemoryCandidateStatus(r.ctx, candidate.ID, model.MemoryCandidateStatusRejected)
			continue
		}
		embed, err := r.svc.llm.Embedding(r.ctx, candidate.Content)
		if err != nil {
			embed = nil
		}
		trustTag := model.MemoryTrustTagApproved
		if decision == model.MemoryCandidateStatusDurable {
			trustTag = model.MemoryTrustTagDurable
		}
		enrichedTags := appendUnique(tags, candidate.CandidateKind, "reviewed", trustTag)
		if _, err := r.svc.repo.AddMemoryChunk(r.ctx, fmt.Sprintf("job:%d:reviewed:%s", r.claim.Job.ID, decision), candidate.CandidateKind, candidate.Content, enrichedTags, embed); err != nil {
			return err
		}
		_ = r.svc.repo.UpdateMemoryCandidateStatus(r.ctx, candidate.ID, decision)
		promoted = append(promoted, candidate.Content)
	}
	summary := strings.Join([]string{fmt.Sprintf("promoted=%d", len(promoted)), fmt.Sprintf("rejected=%d", len(rejected))}, "\n")
	return r.complete("memory_review", summary, summary)
}

func (r *nativeRuntimeV3) runFinalize() error {
	draft, _ := r.readResponseDraftArtifact()
	verificationArtifact, _ := r.readVerificationArtifact()
	final := strings.TrimSpace(draft.Response)
	if final == "" {
		final = strings.TrimSpace(r.contexts["analysis"])
	}
	if len(verificationArtifact.UnsupportedClaims) > 0 {
		prompt := strings.Join([]string{
			"Revise the response to clearly qualify or remove unsupported claims. Keep supported substance.",
			promptBlock("Response Draft", final),
			promptBlock("Unsupported Claims", strings.Join(verificationArtifact.UnsupportedClaims, "\n")),
		}, "\n\n")
		modelName := r.svc.specialistModel(r.claim.Job, specialist.RoleReviewVerificationSpecialist, r.svc.models.Analyze)
		if revised, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, "v3_finalize_rewrite", modelName, prompt); err == nil && strings.TrimSpace(revised) != "" {
			final = strings.TrimSpace(revised)
		}
	}
	return r.complete("response", final, final)
}

func (r *nativeRuntimeV3) complete(contextKey, output, contextValue string) error {
	output = strings.TrimSpace(output)
	contextValue = strings.TrimSpace(contextValue)
	if contextValue == "" {
		contextValue = output
	}
	r.contexts[contextKey] = contextValue
	return r.svc.repo.CompleteStep(r.ctx, r.claim.Step.ID, output, contextKey, contextValue)
}

func (r *nativeRuntimeV3) writeArtifact(kind string, payload any) error {
	env, err := artifacts.MarshalPayload(kind, "1", payload)
	if err != nil {
		return err
	}
	env.JobID = r.claim.Job.ID
	env.StepID = r.claim.Step.ID
	return r.svc.repo.WriteArtifact(r.ctx, env)
}

func (r *nativeRuntimeV3) writeEvidence(record evidence.Record) error {
	record.JobID = r.claim.Job.ID
	record.StepID = r.claim.Step.ID
	return r.svc.repo.WriteEvidence(r.ctx, record)
}

func (r *nativeRuntimeV3) readPlanArtifact() (artifacts.PlanArtifact, bool) {
	return readArtifactPayload[artifacts.PlanArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindPlan)
}

func (r *nativeRuntimeV3) readCapabilityAudit() (artifacts.CapabilityAuditArtifact, bool) {
	return readArtifactPayload[artifacts.CapabilityAuditArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindCapabilityAudit)
}

func (r *nativeRuntimeV3) readWorkspaceArtifact() (artifacts.WorkspaceArtifact, bool) {
	return readArtifactPayload[artifacts.WorkspaceArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindWorkspace)
}

func (r *nativeRuntimeV3) readRetrievalArtifact() (artifacts.RetrievalArtifact, bool) {
	return readArtifactPayload[artifacts.RetrievalArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindRetrieval)
}

func (r *nativeRuntimeV3) readWebArtifact() (artifacts.WebEvidenceArtifact, bool) {
	return readArtifactPayload[artifacts.WebEvidenceArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindWebEvidence)
}

func (r *nativeRuntimeV3) readAnalysisArtifact() (artifacts.AnalysisArtifact, bool) {
	return readArtifactPayload[artifacts.AnalysisArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindAnalysis)
}

func (r *nativeRuntimeV3) readResponseDraftArtifact() (artifacts.ResponseDraftArtifact, bool) {
	return readArtifactPayload[artifacts.ResponseDraftArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindResponseDraft)
}

func (r *nativeRuntimeV3) readVerificationArtifact() (artifacts.VerificationArtifact, bool) {
	return readArtifactPayload[artifacts.VerificationArtifact](r.ctx, r.svc.repo, r.claim.Job.ID, artifacts.KindVerification)
}

func decodeToolOutput[T any](result toolruntime.Result) (T, error) {
	var zero T
	raw, err := json.Marshal(result.Output)
	if err != nil {
		return zero, err
	}
	var payload T
	if err := json.Unmarshal(raw, &payload); err != nil {
		return zero, err
	}
	return payload, nil
}

func (r *nativeRuntimeV3) inferRequiredTools() []string {
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
	instruction := strings.ToLower(strings.TrimSpace(r.claim.Job.Instruction + "\n" + r.contexts["user_feedback"]))
	if containsAnyToken(instruction, "code", "repo", "repository", "file", "golang", "go", "laravel", "project", "workspace") {
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
	sort.Strings(out)
	return out
}

func (r *nativeRuntimeV3) collectSubtaskOutputs() []string {
	out := make([]string, 0, 6)
	for _, ctxItem := range r.claim.Contexts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctxItem.Key)), "subtask:") {
			value := strings.TrimSpace(ctxItem.Value)
			if value == "" {
				continue
			}
			out = append(out, value)
		}
	}
	return out
}

func (r *nativeRuntimeV3) collectDelegatedObjectives() string {
	parts := make([]string, 0, 6)
	for _, ctxItem := range r.claim.Contexts {
		if strings.EqualFold(strings.TrimSpace(ctxItem.Key), "subtask_objective") {
			parts = append(parts, strings.TrimSpace(ctxItem.Value))
		}
	}
	return strings.Join(parts, "\n")
}

func readArtifactPayload[T any](ctx context.Context, repo *queue.Repository, jobID int64, kind string) (T, bool) {
	var zero T
	env, ok, err := repo.LatestArtifact(ctx, jobID, kind)
	if err != nil || !ok {
		return zero, false
	}
	var payload T
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return zero, false
	}
	return payload, true
}

func summarizeCapabilityAudit(audit artifacts.CapabilityAuditArtifact) string {
	return strings.Join([]string{
		"allowed_tools=" + csvOrNone(audit.AllowedTools),
		"available_tools=" + csvOrNone(audit.AvailableTools),
		"missing_tools=" + csvOrNone(audit.MissingTools),
		fmt.Sprintf("workspace_ok=%t", audit.WorkspaceOK),
		fmt.Sprintf("web_search_ok=%t", audit.WebSearchOK),
	}, "\n")
}

func filterDelegatedSubtasks(subtasks []artifacts.Subtask) []artifacts.Subtask {
	out := make([]artifacts.Subtask, 0, len(subtasks))
	seen := map[string]struct{}{}
	for _, subtask := range subtasks {
		kind := strings.ToLower(strings.TrimSpace(subtask.Kind))
		if kind == "respond" || kind == "verify" || kind == "memory_review" {
			continue
		}
		key := kind + "::" + strings.ToLower(strings.TrimSpace(subtask.Objective))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, subtask)
		if len(out) >= maxDelegatedSubtasks {
			break
		}
	}
	return out
}

func joinSubtaskObjectives(subtasks []artifacts.Subtask) string {
	parts := make([]string, 0, len(subtasks))
	for _, subtask := range subtasks {
		if strings.TrimSpace(subtask.Objective) == "" {
			continue
		}
		parts = append(parts, subtask.Objective)
	}
	return strings.Join(parts, "\n")
}

func planSummary(plan artifacts.PlanArtifact) string {
	parts := []string{"goal=" + plan.Goal}
	for _, subtask := range plan.Subtasks {
		parts = append(parts, fmt.Sprintf("- [%s] %s", subtask.Kind, subtask.Objective))
	}
	return strings.Join(parts, "\n")
}

func needsExternalResearch(plan artifacts.PlanArtifact, instruction string, contexts map[string]string) bool {
	if required, ok := plan.Constraints["needs_external_info"].(bool); ok && required {
		return true
	}
	return shouldUseWebSearch(instruction, contexts)
}

func collectAmbiguities(instruction string) []string {
	out := make([]string, 0, 3)
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if strings.Contains(lower, "latest") || strings.Contains(lower, "current") || strings.Contains(lower, "recent") {
		out = append(out, "request depends on freshness")
	}
	if strings.Contains(lower, "this") || strings.Contains(lower, "that") {
		out = append(out, "contains pronouns that may depend on prior context")
	}
	if len(lower) < 18 {
		out = append(out, "very short request may need interpretation")
	}
	return out
}

func collectConstraints(instruction, feedback string) []string {
	out := make([]string, 0, 6)
	joined := strings.ToLower(strings.TrimSpace(instruction + "\n" + feedback))
	if codeOnlyPreferencePattern.MatchString(joined) {
		out = append(out, "prefer code-only output")
	}
	if strings.Contains(joined, "concise") || strings.Contains(joined, "brief") {
		out = append(out, "prefer concise response")
	}
	if strings.Contains(joined, "cite") || strings.Contains(joined, "source") {
		out = append(out, "explicit sourcing requested")
	}
	if strings.Contains(joined, "don't use") && strings.Contains(joined, "memory") {
		out = append(out, "avoid stale memory")
	}
	return out
}

func requiresActionHeuristically(instruction string) bool {
	lower := strings.ToLower(strings.TrimSpace(instruction))
	return containsAnyToken(lower, "build", "fix", "analyze", "compare", "refactor", "plan", "do", "create", "write", "search")
}

func shouldUseWebSearch(instruction string, contexts ...map[string]string) bool {
	if webSearchKeywordPattern.MatchString(strings.ToLower(strings.TrimSpace(instruction))) || explicitWebRequestPattern.MatchString(strings.ToLower(strings.TrimSpace(instruction))) {
		return true
	}
	for _, ctx := range contexts {
		if strings.TrimSpace(ctx["web_search"]) != "" {
			return true
		}
	}
	return false
}

func reviewMemoryCandidate(candidate model.MemoryCandidate, job model.Job) string {
	content := strings.TrimSpace(candidate.Content)
	if len(content) < 18 {
		return model.MemoryCandidateStatusRejected
	}
	instruction := strings.ToLower(strings.TrimSpace(job.Instruction))
	contentLower := strings.ToLower(content)
	if strings.Contains(contentLower, "maybe") || strings.Contains(contentLower, "might") || strings.Contains(contentLower, "probably") {
		return model.MemoryCandidateStatusRejected
	}
	groundedInInstruction := candidateProvenanceBool(candidate, "grounded_in_instruction")
	if strings.Contains(instruction, contentLower) {
		groundedInInstruction = true
	}
	switch candidate.CandidateKind {
	case model.MemoryKindInstruction, model.MemoryKindProcedural:
		if groundedInInstruction && candidate.Confidence >= 0.96 {
			return model.MemoryCandidateStatusDurable
		}
		if groundedInInstruction && candidate.Confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	case model.MemoryKindPreference:
		if groundedInInstruction || candidate.Confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	case model.MemoryKindReference:
		if candidate.Confidence >= 0.9 {
			return model.MemoryCandidateStatusApproved
		}
	}
	return model.MemoryCandidateStatusRejected
}

func candidateProvenanceBool(candidate model.MemoryCandidate, key string) bool {
	if len(candidate.Provenance) == 0 || !json.Valid(candidate.Provenance) {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.Provenance, &payload); err != nil {
		return false
	}
	raw, ok := payload[strings.TrimSpace(key)]
	if !ok {
		return false
	}
	value, ok := raw.(bool)
	return ok && value
}

func extractBlockingBullets(summary string) []string {
	lines := strings.Split(summary, "\n")
	out := make([]string, 0, 4)
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimLeft(line, "-*•0123456789. "))
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "block") || strings.Contains(lower, "risk") || strings.Contains(lower, "missing") {
			out = append(out, clean)
		}
	}
	return firstNStrings(out, 6)
}

func extractAssumptions(summary string) []string {
	lines := strings.Split(summary, "\n")
	out := make([]string, 0, 4)
	for _, line := range lines {
		clean := strings.TrimSpace(strings.TrimLeft(line, "-*•0123456789. "))
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "assum") || strings.Contains(lower, "likely") || strings.Contains(lower, "appears") {
			out = append(out, clean)
		}
	}
	return firstNStrings(out, 6)
}

func firstNStrings(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(items), max))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if len(out) >= max {
			break
		}
	}
	return out
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
