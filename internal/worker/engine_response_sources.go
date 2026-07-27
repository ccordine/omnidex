package worker

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/chat"
	"github.com/gryph/omnidex/internal/model"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func stripSourcesSectionFromResponse(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		if sourceSectionPattern.MatchString(strings.TrimSpace(line)) {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}
	return strings.TrimSpace(text)
}

func trimLikelyProseBoundaries(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}

	start := 0
	for start < len(lines) {
		line := strings.TrimSpace(lines[start])
		if line == "" {
			start++
			continue
		}
		if looksLikeCodeLine(line) {
			break
		}
		if isLikelyProseLine(line) {
			start++
			continue
		}
		break
	}

	end := len(lines) - 1
	for end >= start {
		line := strings.TrimSpace(lines[end])
		if line == "" {
			end--
			continue
		}
		if looksLikeCodeLine(line) {
			break
		}
		if isLikelyProseLine(line) {
			end--
			continue
		}
		break
	}

	if end < start {
		return nil
	}
	return lines[start : end+1]
}

func looksLikeCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)

	if strings.HasPrefix(trimmed, "<") ||
		strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "}") ||
		strings.HasPrefix(trimmed, "[") ||
		strings.HasPrefix(trimmed, "]") ||
		strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "#!") {
		return true
	}

	for _, prefix := range []string{
		"const ", "let ", "var ", "function ", "class ", "interface ", "type ",
		"def ", "import ", "from ", "package ", "func ", "return ", "if ", "for ",
		"while ", "switch ", "case ", "public ", "private ", "protected ",
		"select ", "insert ", "update ", "delete ", "create ", "alter ", "with ",
		"echo ", "set ", "export ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	if strings.HasSuffix(trimmed, ";") {
		return true
	}
	if strings.Contains(trimmed, "</") || strings.Contains(trimmed, "/>") {
		return true
	}
	if strings.Contains(trimmed, " = ") && strings.ContainsAny(trimmed, "{}()[]<>") {
		return true
	}
	if strings.Contains(trimmed, " := ") {
		return true
	}

	return false
}

func isLikelyProseLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)

	if sourceSectionPattern.MatchString(trimmed) {
		return true
	}
	for _, prefix := range []string{
		"here is", "here's", "this is", "the following", "output:", "note:", "notes:",
		"explanation:", "summary:", "to proceed", "let me know", "if you", "i can",
		"i'm ", "i am ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if strings.HasSuffix(trimmed, ".") && len(strings.Fields(trimmed)) >= 5 && !strings.ContainsAny(trimmed, "{}[]();=<>\t") {
		return true
	}
	return false
}

func isDeterministicLocalActionReviewInstruction(instruction string) bool {
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "deterministic post-action review step (required):")
}

func isLowSignalChatInstruction(instruction string, pipeline string) bool {
	return chat.IsLowSignal(instruction, pipeline)
}

func webSearchMode(metadata json.RawMessage) string {
	for _, key := range []string{"web_search", "web", "search"} {
		if value, ok := metadataValue(metadata, key); ok {
			switch typed := value.(type) {
			case bool:
				if typed {
					return "force"
				}
				return "off"
			case string:
				mode := strings.ToLower(strings.TrimSpace(typed))
				switch mode {
				case "on", "force", "enabled", "true":
					return "force"
				case "off", "disabled", "false", "skip":
					return "off"
				}
			}
		}
	}

	return "auto"
}

func resolveApprovalMode(metadata json.RawMessage) string {
	raw := strings.ToLower(strings.TrimSpace(metadataString(metadata, "approval_mode")))
	switch raw {
	case "force", "on", "true":
		return "force"
	case "off", "false", "disabled":
		return "off"
	default:
		return "auto"
	}
}

func resolveVerificationMode(metadata json.RawMessage) string {
	raw := strings.ToLower(strings.TrimSpace(metadataString(metadata, "verification_mode")))
	switch raw {
	case "force", "on", "true":
		return "force"
	case "off", "false", "disabled":
		return "off"
	default:
		return "auto"
	}
}

func detectRiskSignals(instruction, plan string) []string {
	combined := strings.ToLower(strings.TrimSpace(instruction + "\n" + plan))
	if combined == "" {
		return nil
	}
	signals := make([]string, 0, 4)
	if riskyActionPattern.MatchString(combined) {
		signals = append(signals, "destructive command/data-loss pattern")
	}
	if strings.Contains(combined, "production") &&
		(strings.Contains(combined, "delete") || strings.Contains(combined, "drop") || strings.Contains(combined, "reset")) {
		signals = append(signals, "production target with destructive intent")
	}
	if strings.Contains(combined, "database") &&
		(strings.Contains(combined, "drop") || strings.Contains(combined, "truncate")) {
		signals = append(signals, "database destructive operation")
	}
	if strings.Contains(combined, "revoke") && strings.Contains(combined, "access") {
		signals = append(signals, "access revocation operation")
	}
	return appendUnique(nil, signals...)
}

func hasExplicitApproval(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "approve") ||
		strings.HasPrefix(normalized, "approved") ||
		strings.HasPrefix(normalized, "yes, proceed") ||
		strings.HasPrefix(normalized, "yes proceed") ||
		strings.Contains(normalized, " i approve")
}

func sessionTag(job model.Job) string {
	raw := strings.TrimSpace(metadataString(job.Metadata, "session_id"))
	if raw == "" {
		return ""
	}
	sanitized := normalizeSessionID(raw)
	if sanitized == "" {
		return ""
	}
	return "session:" + sanitized
}

func projectTag(job model.Job) string {
	location := strings.TrimSpace(metadataString(job.Metadata, "client_cwd"))
	if location == "" {
		location = strings.TrimSpace(metadataString(job.Metadata, "host_env_cwd"))
	}
	if location == "" {
		return ""
	}

	clean := filepath.Clean(location)
	base := normalizeSessionID(filepath.Base(clean))
	if base == "" {
		base = "workspace"
	}

	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(clean))))
	suffix := hex.EncodeToString(sum[:4])
	return "project:" + base + "-" + suffix
}

func memoryScopeTags(job model.Job, base []string) []string {
	tags := appendUnique(nil, base...)
	if project := projectTag(job); project != "" {
		tags = appendUnique([]string{project}, tags...)
	}
	if session := sessionTag(job); session != "" {
		tags = appendUnique([]string{session}, tags...)
	}
	return tags
}

func normalizeSessionID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if prevDash {
			continue
		}
		b.WriteRune('-')
		prevDash = true
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

func metadataString(metadata json.RawMessage, key string) string {
	value, ok := metadataValue(metadata, key)
	if !ok {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func metadataModel(job model.Job, key, fallback string) string {
	value := strings.TrimSpace(metadataString(job.Metadata, key))
	if value == "" {
		return fallback
	}
	return value
}

func normalizePlanText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(value[start : end+1])
	}

	return value
}

func planNeedsExternalInfo(plan string) (bool, bool) {
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return false, false
	}

	start := strings.Index(plan, "{")
	end := strings.LastIndex(plan, "}")
	if start < 0 || end <= start {
		return false, false
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(plan[start:end+1]), &payload); err != nil {
		return false, false
	}

	value, ok := payload["needs_external_info"]
	if !ok {
		return false, false
	}

	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		clean := strings.ToLower(strings.TrimSpace(typed))
		switch clean {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}

	return false, false
}

func planClarificationQuestion(plan string) string {
	questions := planClarificationQuestions(plan, 1)
	if len(questions) == 0 {
		return ""
	}
	return questions[0]
}

func planClarificationQuestions(plan string, limit int) []string {
	payload, ok := parsePlanPayload(plan)
	if !ok {
		return nil
	}

	value, ok := payload["clarifications"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = len(items)
	}

	out := make([]string, 0, minInt(limit, len(items)))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func formatClarificationQuestions(questions []string) string {
	if len(questions) == 0 {
		return ""
	}
	if len(questions) == 1 {
		return questions[0]
	}

	parts := make([]string, 0, len(questions))
	for i, question := range questions {
		parts = append(parts, fmt.Sprintf("%d) %s", i+1, strings.TrimSpace(question)))
	}
	return "Please confirm before I continue: " + strings.Join(parts, " ")
}

func clearPlanClarifications(plan string) string {
	payload, ok := parsePlanPayload(plan)
	if !ok {
		return plan
	}
	payload["clarifications"] = []string{}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return plan
	}
	return string(encoded)
}

func forcePlanNeedsExternalInfo(plan string) string {
	payload, ok := parsePlanPayload(plan)
	if !ok {
		return plan
	}
	payload["needs_external_info"] = true
	encoded, err := json.Marshal(payload)
	if err != nil {
		return plan
	}
	return string(encoded)
}

func planningPassCount(job model.Job) int {
	keys := []string{"planning_passes", "planner_passes", "plan_candidates", "planning_iterations"}
	for _, key := range keys {
		value := metadataInt(job.Metadata, key, 0)
		if value <= 0 {
			continue
		}
		if value > maxPlanningPasses {
			return maxPlanningPasses
		}
		return value
	}
	return defaultPlanningPasses
}

func summarizePlanCandidate(pass int, plan string) string {
	needsExternal, _ := planNeedsExternalInfo(plan)
	return fmt.Sprintf("candidate=%d tasks=%d needs_external=%t chars=%d", pass, parsePlanTaskCount(plan), needsExternal, len(strings.TrimSpace(plan)))
}

func (s *Service) selectBestPlanCandidateIndex(
	ctx context.Context,
	stepID int64,
	job model.Job,
	modelName string,
	feedback string,
	actionCatalog string,
	candidates []string,
	forceFreshExternal bool,
) (int, string, error) {
	if len(candidates) == 0 {
		return -1, "", fmt.Errorf("plan selection requires at least one candidate")
	}
	if len(candidates) == 1 {
		return 0, "single_candidate", nil
	}

	_, heuristicReason := heuristicPlanSelection(candidates, job.Instruction, forceFreshExternal)
	promptLines := []string{
		"You are selecting the best execution plan candidate.",
		antiRoleplayInstructionForPipeline(job.Pipeline),
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", job.Instruction, feedback),
		`Return JSON only: {"best_index":1,"reason":"..."}`,
		"best_index is 1-based.",
		"Selection criteria in strict order:",
		"1) direct relevance to USER_INSTRUCTION and USER_FEEDBACK",
		"2) grounded in ACTION_CATALOG actions; no invented capabilities",
		"3) low hallucination risk (no unsupported assumptions)",
		"4) convenience and executability (clear micro-steps, low unnecessary clarification)",
		"5) needs_external_info alignment with explicit freshness/web requirements",
		promptBlock("USER_INSTRUCTION", job.Instruction),
		promptBlock("USER_FEEDBACK", feedback),
		promptBlock("FORCE_FRESH_EXTERNAL", strconv.FormatBool(forceFreshExternal)),
		promptBlock("ACTION_CATALOG", trimForBudget(actionCatalog, 2400)),
		promptUserAnchor("end", job.Instruction, feedback),
		"Final grounding check: rank candidates by AUTHORITATIVE_USER_INSTRUCTION_END.",
	}
	for i, candidate := range candidates {
		promptLines = append(promptLines, promptBlock(fmt.Sprintf("PLAN_CANDIDATE_%d", i+1), trimForBudget(candidate, 2600)))
	}
	raw, err := s.llmGenerateWithTrace(
		ctx,
		stepID,
		"plan_candidate_selection",
		modelName,
		strings.Join(promptLines, "\n\n"),
	)
	if err != nil {
		return -1, "", fmt.Errorf("plan candidate ranking failed: %w", err)
	}
	if idx, reason, ok := parseBestPlanIndex(raw, len(candidates)); ok {
		note := strings.TrimSpace(reason)
		if note == "" {
			note = "llm_selected"
		}
		return idx, trimForBudget("llm_selected: "+note+" | "+heuristicReason, 1200), nil
	}
	return -1, "", fmt.Errorf("plan candidate ranking returned invalid output: %q", trimForBudget(raw, 300))
}

func parseBestPlanIndex(raw string, total int) (int, string, bool) {
	payload := strings.TrimSpace(raw)
	if payload == "" || total <= 0 {
		return 0, "", false
	}
	if !strings.HasPrefix(payload, "{") {
		start := strings.Index(payload, "{")
		end := strings.LastIndex(payload, "}")
		if start >= 0 && end > start {
			payload = payload[start : end+1]
		}
	}
	var decoded struct {
		BestIndex int    `json:"best_index"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
		if decoded.BestIndex >= 1 && decoded.BestIndex <= total {
			return decoded.BestIndex - 1, strings.TrimSpace(decoded.Reason), true
		}
	}
	match := regexp.MustCompile(`(?i)best[_ ]?index[^0-9]*(\d+)`).FindStringSubmatch(payload)
	if len(match) == 2 {
		n, err := strconv.Atoi(strings.TrimSpace(match[1]))
		if err == nil && n >= 1 && n <= total {
			return n - 1, "", true
		}
	}
	return 0, "", false
}

func heuristicPlanSelection(candidates []string, instruction string, forceFreshExternal bool) (int, string) {
	if len(candidates) == 0 {
		return 0, "no_candidates"
	}
	scores := make([]planCandidateScore, 0, len(candidates))
	for i, candidate := range candidates {
		score, reason := scorePlanCandidate(candidate, instruction, forceFreshExternal)
		scores = append(scores, planCandidateScore{
			Index:  i,
			Score:  score,
			Reason: reason,
		})
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].Index < scores[j].Index
		}
		return scores[i].Score > scores[j].Score
	})
	best := scores[0]
	reason := fmt.Sprintf("heuristic_score=%d candidate=%d details=%s", best.Score, best.Index+1, best.Reason)
	if len(scores) > 1 {
		reason = fmt.Sprintf("%s runner_up=%d(%d)", reason, scores[1].Index+1, scores[1].Score)
	}
	return best.Index, reason
}

func scorePlanCandidate(plan string, instruction string, forceFreshExternal bool) (int, string) {
	score := 0
	reasons := make([]string, 0, 8)
	payload, parsed := parsePlanPayload(plan)
	if parsed {
		score += 30
		reasons = append(reasons, "json=ok")
	} else {
		score -= 35
		reasons = append(reasons, "json=invalid")
	}

	taskCount := parsePlanTaskCount(plan)
	switch {
	case taskCount >= 8 && taskCount <= 14:
		score += 22
	case taskCount >= 4 && taskCount <= 18:
		score += 14
	case taskCount >= 1:
		score += 8
	default:
		score -= 20
	}
	reasons = append(reasons, fmt.Sprintf("tasks=%d", taskCount))

	clarCount := len(planClarificationQuestions(plan, 8))
	switch {
	case clarCount == 0:
		score += 8
	case clarCount == 1:
		score += 4
	default:
		score -= 6
	}
	reasons = append(reasons, fmt.Sprintf("clarifications=%d", clarCount))

	doneWhenCount := len(planFieldStrings(payload, "done_when"))
	switch {
	case doneWhenCount >= 2 && doneWhenCount <= 4:
		score += 8
	case doneWhenCount == 1:
		score += 4
	case doneWhenCount == 0:
		score -= 6
	}
	reasons = append(reasons, fmt.Sprintf("done_when=%d", doneWhenCount))

	needsExternal, decided := planNeedsExternalInfo(plan)
	if forceFreshExternal {
		if needsExternal {
			score += 20
			reasons = append(reasons, "external=aligned")
		} else {
			score -= 30
			reasons = append(reasons, "external=misaligned")
		}
	} else if decided && !needsExternal {
		score += 4
	}

	requiredToolsCount := len(planFieldStrings(payload, "required_tools"))
	if requiredToolsCount > 0 {
		score += 3
	}
	reasons = append(reasons, fmt.Sprintf("required_tools=%d", requiredToolsCount))

	relevance := tokenOverlapScore(instruction, planRelevanceText(plan, payload))
	score += relevance
	reasons = append(reasons, fmt.Sprintf("relevance=%d", relevance))

	return score, strings.Join(reasons, ",")
}

func planFieldStrings(payload map[string]any, key string) []string {
	if len(payload) == 0 {
		return nil
	}
	value, ok := payload[key]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}
