package worker

import (
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"sort"
	"strconv"
	"strings"
)

func planRelevanceText(plan string, payload map[string]any) string {
	if len(payload) == 0 {
		return plan
	}
	segments := []string{}
	if goal, ok := payload["goal"].(string); ok {
		goal = strings.TrimSpace(goal)
		if goal != "" {
			segments = append(segments, goal)
		}
	}
	segments = append(segments, planFieldStrings(payload, "tasks")...)
	segments = append(segments, planFieldStrings(payload, "done_when")...)
	if len(segments) == 0 {
		return plan
	}
	return strings.Join(segments, "\n")
}

func tokenOverlapScore(left, right string) int {
	leftTokens := significantTokens(left)
	if len(leftTokens) == 0 {
		return 0
	}
	rightTokens := significantTokens(right)
	if len(rightTokens) == 0 {
		return -12
	}
	rightSet := map[string]struct{}{}
	for _, token := range rightTokens {
		rightSet[token] = struct{}{}
	}
	overlap := 0
	for _, token := range leftTokens {
		if _, ok := rightSet[token]; ok {
			overlap++
		}
	}
	if overlap == 0 {
		if len(leftTokens) >= 4 {
			return -12
		}
		return -6
	}
	score := (overlap * 22) / len(leftTokens)
	if score > 22 {
		score = 22
	}
	if score < 3 && len(leftTokens) >= 6 {
		return -3
	}
	return score
}

func significantTokens(value string) []string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return nil
	}
	matches := tokenWordPattern.FindAllString(lower, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, token := range matches {
		if isStopwordToken(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func isStopwordToken(token string) bool {
	switch token {
	case "the", "and", "for", "with", "this", "that", "from", "into", "when", "where", "what",
		"which", "your", "ours", "their", "then", "than", "have", "has", "had", "will", "would",
		"should", "could", "about", "need", "must", "please", "just", "user", "task", "plan",
		"step", "steps", "true", "false", "auto", "mode":
		return true
	default:
		return false
	}
}

func reviewAlwaysEnabled(job model.Job) bool {
	for _, key := range []string{"review_always", "verification_review", "hallucination_review"} {
		raw := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, key)))
		switch raw {
		case "on", "true", "enabled", "force", "always":
			return true
		case "off", "false", "disabled", "never":
			return false
		}
		if value, ok := metadataValue(job.Metadata, key); ok {
			if typed, ok := value.(bool); ok {
				return typed
			}
		}
	}
	return true
}

func enforceGroundingReview(
	outcome verificationOutcome,
	job model.Job,
	response string,
	contexts map[string]string,
	report testReport,
) (verificationOutcome, []string) {
	signals := detectGroundingSignals(job, response, contexts, report)
	if len(signals) == 0 {
		return outcome, nil
	}

	updated := outcome
	updated.Gaps = dedupeStrings(append(updated.Gaps, signals...))
	if updated.Status == "pass" || (updated.Status == "blocked" && strings.TrimSpace(updated.CannotCompleteReason) == "") {
		updated.Status = "retry"
	}
	if strings.TrimSpace(updated.Summary) == "" || updated.Status == "retry" {
		updated.Summary = "review flagged unsupported or weakly related claims"
	}
	return updated, signals
}

func detectGroundingSignals(job model.Job, response string, contexts map[string]string, report testReport) []string {
	text := strings.TrimSpace(response)
	if text == "" && len(missingRequiredActionsForVerification(job, contexts)) == 0 {
		return nil
	}
	lower := strings.ToLower(text)
	signals := make([]string, 0, 4)

	if webExecutionClaimPattern.MatchString(text) && !hasWebSearchContext(contexts["web_search"]) {
		signals = append(signals, "claims web search execution without web_search evidence in this run")
	}
	if webEvidenceClaimPattern.MatchString(text) && !hasWebSearchContext(contexts["web_search"]) {
		signals = append(signals, "cites online/web evidence without web_search context in this run")
	}
	if executionClaimPattern.MatchString(text) && report.Attempted == 0 {
		signals = append(signals, "claims command/action execution without execution evidence in this run")
	}
	if report.Attempted == 0 {
		if strings.Contains(lower, "tests passed") ||
			strings.Contains(lower, "all tests pass") ||
			strings.Contains(lower, "i ran tests") ||
			strings.Contains(lower, "we ran tests") {
			signals = append(signals, "claims test execution/results without executed tests")
		}
	}
	for _, action := range missingRequiredActionsForVerification(job, contexts) {
		signals = append(signals, "required action missing in this run: "+action)
	}
	if responseSeemsOffTopic(job.Instruction, text) {
		signals = append(signals, "response appears weakly related to the user instruction")
	}

	return dedupeStrings(signals)
}

func buildVerificationActionAudit(job model.Job, contexts map[string]string) verificationActionAudit {
	responseAction := responseContextKeyForPipeline(job.Pipeline)
	defs := []struct {
		action string
		key    string
	}{
		{action: "plan", key: "plan"},
		{action: "tooling", key: "tooling"},
		{action: "workspace_scan", key: "workspace"},
		{action: "tag", key: "tags"},
		{action: "retrieve", key: "retrieval"},
		{action: "web_search", key: "web_search"},
		{action: "analyze", key: "analyzer"},
		{action: responseAction, key: responseAction},
	}

	lines := make([]string, 0, len(defs)+3)
	for _, def := range defs {
		status, detail := classifyActionExecution(def.action, contexts[def.key])
		line := def.action + "=" + status
		if strings.TrimSpace(detail) != "" {
			line += " detail=" + safeLine(detail, "n/a")
		}
		lines = append(lines, line)
	}

	requiredWeb := verificationRequiresWebSearch(job, contexts)
	missingRequired := missingRequiredActionsForVerification(job, contexts)
	lines = append(lines, fmt.Sprintf("required_web_search=%t", requiredWeb))
	if len(missingRequired) == 0 {
		lines = append(lines, "required_missing_actions=(none)")
	} else {
		lines = append(lines, "required_missing_actions="+strings.Join(missingRequired, ","))
	}

	return verificationActionAudit{
		Report:          strings.Join(lines, "\n"),
		MissingRequired: missingRequired,
	}
}

func classifyActionExecution(action string, value string) (string, string) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return "missing", "no context recorded"
	}

	switch action {
	case "web_search":
		if hasWebSearchContext(clean) {
			return "executed", summarizeActionContextDetail(clean, 180)
		}
		return "skipped", summarizeActionContextDetail(clean, 180)
	case "workspace_scan":
		if hasWorkspaceContext(clean) {
			return "executed", summarizeActionContextDetail(clean, 180)
		}
		return "skipped", summarizeActionContextDetail(clean, 180)
	case "retrieve":
		if hasRetrievalContext(clean) {
			return "executed", summarizeActionContextDetail(clean, 180)
		}
		return "skipped", summarizeActionContextDetail(clean, 180)
	case "tooling":
		if hasToolingContext(clean) {
			return "executed", summarizeActionContextDetail(clean, 180)
		}
		return "skipped", summarizeActionContextDetail(clean, 180)
	}

	lower := strings.ToLower(clean)
	if strings.Contains(lower, "skipped") || strings.Contains(lower, "unavailable") || strings.Contains(lower, "not required") {
		return "skipped", summarizeActionContextDetail(clean, 180)
	}
	return "executed", summarizeActionContextDetail(clean, 180)
}

func summarizeActionContextDetail(value string, maxChars int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		return trimForBudget(clean, maxChars)
	}
	return ""
}

func verificationRequiresWebSearch(job model.Job, contexts map[string]string) bool {
	feedback := strings.TrimSpace(strings.Join([]string{
		contexts["user_feedback"],
		metadataString(job.Metadata, "replan_feedback"),
	}, "\n"))
	if shouldForceFreshWebSearch(job.Instruction, feedback) {
		return true
	}
	needsExternal, decided := planNeedsExternalInfo(contexts["plan"])
	if decided && needsExternal {
		return true
	}
	if isTimeSensitiveInstruction(job.Instruction) && !isLocalClockOnlyInstruction(job.Instruction) {
		return webSearchMode(job.Metadata) != "off"
	}
	return false
}

func missingRequiredActionsForVerification(job model.Job, contexts map[string]string) []string {
	missing := make([]string, 0, 2)
	if verificationRequiresWebSearch(job, contexts) && !hasWebSearchContext(contexts["web_search"]) {
		missing = append(missing, "web_search")
	}
	return missing
}

func autoVerifyReplanFeedback(
	job model.Job,
	contexts map[string]string,
	priorContexts []model.StepContext,
	outcome verificationOutcome,
) (string, []string, bool) {
	if !persistentExecutionEnabled(job) {
		return "", nil, false
	}
	status := strings.ToLower(strings.TrimSpace(outcome.Status))
	if status == "pass" {
		return "", nil, false
	}
	missing := missingRequiredActionsForVerification(job, contexts)
	if countAutoVerifyReplans(priorContexts) >= maxAutoVerifyReplans {
		return "", missing, false
	}
	return buildAutoVerifyReplanFeedback(job, contexts, missing, outcome), missing, true
}

func buildAutoVerifyReplanFeedback(job model.Job, contexts map[string]string, missing []string, outcome verificationOutcome) string {
	audit := trimForBudget(strings.TrimSpace(contexts["verification_action_audit"]), 500)
	audit = strings.ReplaceAll(audit, "\r\n", "\n")
	audit = strings.ReplaceAll(audit, "\n", " | ")
	if strings.TrimSpace(audit) == "" {
		audit = "(no verification action audit captured)"
	}
	gaps := trimForBudget(strings.Join(outcome.Gaps, " | "), 500)
	if strings.TrimSpace(gaps) == "" {
		gaps = "(no explicit gaps provided)"
	}
	missingText := strings.Join(missing, ",")
	if strings.TrimSpace(missingText) == "" {
		missingText = "(none)"
	}
	lines := []string{
		autoVerifyReplanMarker + ": restart from planning because dual verification did not confirm completion.",
		"replan_mode=objective_recovery",
		"verification_status=" + strings.ToLower(strings.TrimSpace(outcome.Status)),
		"missing_required_actions=" + missingText,
		"instruction=" + trimForBudget(strings.TrimSpace(job.Instruction), 500),
		"current_state_action_audit=" + audit,
	}
	if summary := strings.TrimSpace(outcome.Summary); summary != "" {
		lines = append(lines, "verification_summary="+trimForBudget(summary, 320))
	}
	lines = append(lines, "verification_gaps="+gaps)
	lines = append(lines,
		"restart_focus=original user objective remains the primary goal",
		"restart_focus=use current state to close verification gaps before final output",
		"restart_guidance=formulate an explicit recovery plan from current state, execute it, then verify objective completion again",
		"restart_guidance=if web_search is required, run it with a focused query and use the retrieved context",
		"restart_guidance=if blocked by tooling or permissions, ask concise clarification with concrete options",
	)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func countAutoVerifyReplans(contexts []model.StepContext) int {
	if len(contexts) == 0 {
		return 0
	}
	count := 0
	for _, value := range collectContextValuesByKey(contexts, "replan_feedback") {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), autoVerifyReplanMarker) {
			count++
		}
	}
	return count
}

func responseSeemsOffTopic(instruction string, response string) bool {
	instruction = strings.TrimSpace(instruction)
	response = strings.TrimSpace(response)
	if instruction == "" || response == "" {
		return false
	}
	if _, needInput := extractNeedInputQuestion(response); needInput {
		return false
	}
	instructionTokens := significantTokens(instruction)
	if len(instructionTokens) < 4 {
		return false
	}
	responseTokens := significantTokens(response)
	if len(responseTokens) == 0 {
		return true
	}
	responseSet := map[string]struct{}{}
	for _, token := range responseTokens {
		responseSet[token] = struct{}{}
	}
	overlap := 0
	for _, token := range instructionTokens {
		if _, ok := responseSet[token]; ok {
			overlap++
		}
	}
	if overlap == 0 {
		return true
	}
	overlapPct := (overlap * 100) / len(instructionTokens)
	return overlapPct < 8 && len(response) > 240
}

func persistentExecutionEnabled(job model.Job) bool {
	keys := []string{"persistent_execution", "no_early_stop", "full_execution", "execution_persistence"}
	for _, key := range keys {
		raw := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, key)))
		switch raw {
		case "on", "true", "enabled", "force", "always":
			return true
		case "off", "false", "disabled", "never":
			return false
		}
		if value, ok := metadataValue(job.Metadata, key); ok {
			if typed, ok := value.(bool); ok {
				return typed
			}
		}
	}
	return false
}

func resolveAutonomyMode(job model.Job) string {
	for _, key := range []string{"autonomy_mode", "autonomy", "autonomous"} {
		raw := strings.ToLower(strings.TrimSpace(metadataString(job.Metadata, key)))
		switch raw {
		case "on", "true", "enabled", "force":
			return "on"
		case "off", "false", "disabled", "strict":
			return "off"
		case "auto":
			if strings.EqualFold(strings.TrimSpace(job.Pipeline), model.PipelineChat) {
				return "on"
			}
			return "off"
		}
	}

	if strings.EqualFold(strings.TrimSpace(job.Pipeline), model.PipelineChat) {
		return "on"
	}
	return "off"
}

func autonomyEnabled(job model.Job) bool {
	return resolveAutonomyMode(job) == "on"
}

func mustAskForClarification(question, instruction string) bool {
	text := strings.ToLower(strings.TrimSpace(question + " " + instruction))
	if text == "" {
		return false
	}

	if riskyActionPattern.MatchString(text) {
		return true
	}

	blockers := []string{
		"production",
		"password",
		"secret",
		"api key",
		"token",
		"credential",
		"billing",
		"payment",
	}
	for _, token := range blockers {
		if strings.Contains(text, token) {
			return true
		}
	}

	return false
}

func parsePlanRequiredTools(plan string) []string {
	payload, ok := parsePlanPayload(plan)
	if !ok {
		return nil
	}

	value, ok := payload["required_tools"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = normalizeToolName(text)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
	}
	return out
}

func pipelinePhaseForAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	action = strings.TrimPrefix(action, "v3_")
	switch action {
	case "intent_parse", "capability_audit", "plan", "planning", "tooling", "workspace_scan", "workspace_research", "tag", "retrieve", "memory_retrieval":
		return "planning"
	case "verify", "verification", "memory_review":
		return "review"
	default:
		return "execution"
	}
}

func parsePlanTaskCount(plan string) int {
	payload, ok := parsePlanPayload(plan)
	if !ok {
		return 0
	}
	value, ok := payload["tasks"]
	if !ok {
		return 0
	}
	items, ok := value.([]any)
	if !ok {
		return 0
	}
	return len(items)
}

func parsePlanPayload(plan string) (map[string]any, bool) {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return nil, false
	}

	start := strings.Index(plan, "{")
	end := strings.LastIndex(plan, "}")
	if start < 0 || end <= start {
		return nil, false
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(plan[start:end+1]), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func metadataBool(metadata json.RawMessage, key string, fallback bool) bool {
	value, ok := metadataValue(metadata, key)
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		clean := strings.ToLower(strings.TrimSpace(typed))
		switch clean {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func metadataInt(metadata json.RawMessage, key string, fallback int) int {
	value, ok := metadataValue(metadata, key)
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func shouldScanWorkspace(instruction string, plan string) bool {
	if codeKeywordPattern.MatchString(strings.ToLower(strings.TrimSpace(instruction))) {
		return true
	}
	planLower := strings.ToLower(strings.TrimSpace(plan))
	if planLower == "" {
		return false
	}
	return strings.Contains(planLower, "file") ||
		strings.Contains(planLower, "code") ||
		strings.Contains(planLower, "repository") ||
		strings.Contains(planLower, "project")
}

func extractNeedInputQuestion(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	matches := needInputPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return "", false
	}
	question := strings.TrimSpace(matches[1])
	if question == "" {
		return "", false
	}
	return question, true
}

func inferRequiredToolsFromInstruction(instruction string) []string {
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if lower == "" {
		return nil
	}

	type toolHint struct {
		tool     string
		triggers []string
	}
	hints := []toolHint{
		{tool: "go", triggers: []string{" go ", " golang ", " go.mod ", " go test", " go build"}},
		{tool: "npm", triggers: []string{" npm ", " package.json ", " node ", " react ", " nextjs ", " next.js"}},
		{tool: "pnpm", triggers: []string{" pnpm "}},
		{tool: "yarn", triggers: []string{" yarn "}},
		{tool: "python", triggers: []string{" python ", " pip ", " requirements.txt ", " pyproject.toml"}},
		{tool: "composer", triggers: []string{" composer ", " postgres ", " php "}},
		{tool: "docker", triggers: []string{" docker ", " container ", " dockerfile ", " compose "}},
		{tool: "git", triggers: []string{" git ", " repository ", " repo ", " branch ", " commit "}},
		{tool: "make", triggers: []string{" makefile ", " make "}},
		{tool: "ffmpeg", triggers: []string{" ffmpeg ", " subtitle ", " video ", " audio "}},
	}

	padded := " " + lower + " "
	out := make([]string, 0, 6)
	seen := map[string]struct{}{}
	for _, hint := range hints {
		for _, trigger := range hint.triggers {
			if strings.Contains(padded, trigger) {
				if _, ok := seen[hint.tool]; ok {
					break
				}
				seen[hint.tool] = struct{}{}
				out = append(out, hint.tool)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func normalizeToolName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = strings.Trim(value, "\"'`[](){} ")
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	if len(parts) > 0 {
		value = parts[0]
	}
	value = strings.Trim(value, ",.;:")
	value = strings.TrimPrefix(value, "`")
	value = strings.TrimSuffix(value, "`")

	aliases := map[string]string{
		"golang":         "go",
		"nodejs":         "node",
		"node.js":        "node",
		"docker-compose": "docker",
		"pip3":           "pip",
	}
	if mapped, ok := aliases[value]; ok {
		return mapped
	}
	return value
}

func detectPackageManager() string {
	candidates := detectPackageManagers()
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}
