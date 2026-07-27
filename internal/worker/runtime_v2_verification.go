package worker

import (
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	runtimev3 "github.com/gryph/omnidex/internal/runtime/v3"
	"github.com/gryph/omnidex/internal/verification"
	"github.com/gryph/omnidex/internal/workspace"
	"sort"
	"strings"
)

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
		evidenceRecords, err := r.svc.repo.ListEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
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
