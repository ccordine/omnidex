package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"sort"
	"strings"
	"time"
)

func (s *Service) inferMemory(ctx context.Context, stepID int64, job model.Job, contexts map[string]string, response string) error {
	if !s.cognition.MemoryInferenceEnabled || s.cognition.MemoryInferenceMaxItems == 0 {
		return nil
	}

	prompt := strings.Join([]string{
		antiRoleplayInstructionForPipeline(job.Pipeline),
		"Extract durable memories from this interaction and return strict JSON only.",
		`Schema: {"procedural":[],"instruction":[],"preference":[]}`,
		"Rules: keep each item concrete, reusable, and concise. Omit empty categories.",
		"Instruction:",
		trimForBudget(job.Instruction, 1200),
		"Assistant Response:",
		trimForBudget(response, 1600),
	}, "\n\n")

	memoryFallback := s.specialistModel(job, specialist.RoleMemoryRetrievalSpecialist, s.models.Memory)
	memoryModel := metadataModel(job, "model_memory", memoryFallback)
	raw, err := s.llmGenerateWithTrace(ctx, stepID, "memory_inference", memoryModel, prompt)
	if err != nil {
		return err
	}

	payload := strings.TrimSpace(raw)
	if !strings.HasPrefix(payload, "{") {
		start := strings.Index(payload, "{")
		end := strings.LastIndex(payload, "}")
		if start >= 0 && end > start {
			payload = payload[start : end+1]
		}
	}

	var inferred inferredMemory
	if err := json.Unmarshal([]byte(payload), &inferred); err != nil {
		return nil
	}

	tags := memoryScopeTags(job, parseTagsCSV(contexts["tags"]))
	if len(tags) == 0 {
		tags = []string{"general"}
	}

	type memoryCandidate struct {
		kind  string
		items []string
	}
	candidates := []memoryCandidate{
		{kind: model.MemoryKindProcedural, items: inferred.Procedural},
		{kind: model.MemoryKindInstruction, items: inferred.Instruction},
		{kind: model.MemoryKindPreference, items: inferred.Preference},
	}

	remaining := s.cognition.MemoryInferenceMaxItems
	if remaining < 0 {
		remaining = 0
	}
	autopromote := metadataBool(job.Metadata, "memory_inference_autopromote", false)
	instructionLower := strings.ToLower(strings.TrimSpace(job.Instruction))

	for _, candidate := range candidates {
		if remaining == 0 {
			break
		}
		for _, item := range candidate.items {
			if remaining == 0 {
				break
			}
			content := strings.TrimSpace(item)
			if len(content) < 16 {
				continue
			}
			confidence := 0.55
			groundedInInstruction := strings.Contains(instructionLower, strings.ToLower(content))
			if groundedInInstruction {
				confidence = 0.92
			}
			status := memoryCandidateStatusForInference(candidate.kind, confidence, groundedInInstruction, autopromote)
			provenanceMap := map[string]any{
				"job_id":                  job.ID,
				"step_id":                 stepID,
				"source":                  "memory_inference",
				"kind":                    candidate.kind,
				"grounded_in_instruction": groundedInInstruction,
				"scope_tags":              append([]string(nil), tags...),
				"status":                  status,
			}
			provenanceRaw, _ := json.Marshal(provenanceMap)
			candidateID, err := s.repo.WriteMemoryCandidate(ctx, model.MemoryCandidate{JobID: job.ID, CandidateKind: candidate.kind, Content: content, Provenance: provenanceRaw, Confidence: confidence, Status: status})
			if err != nil {
				return err
			}
			if status == model.MemoryCandidateStatusApproved {
				embed, err := s.llm.Embedding(ctx, content)
				if err != nil {
					embed = nil
				}
				enrichedTags := appendUnique(tags, candidate.kind, model.MemoryTrustTagApproved)
				if _, err := s.repo.AddMemoryChunk(ctx, fmt.Sprintf("job:%d:inferred:approved", job.ID), candidate.kind, content, enrichedTags, embed); err != nil {
					return err
				}
				_ = s.repo.UpdateMemoryCandidateStatus(ctx, candidateID, model.MemoryCandidateStatusApproved)
			}
			remaining--
		}
	}

	return nil
}

func appendUnique(base []string, values ...string) []string {
	out := make([]string, 0, len(base)+len(values))
	seen := make(map[string]struct{}, len(base)+len(values))

	for _, item := range base {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	for _, item := range values {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	return out
}

func (s *Service) persistMemory(ctx context.Context, job model.Job, contexts map[string]string, response string) error {
	tags := memoryScopeTags(job, parseTagsCSV(contexts["tags"]))
	if len(tags) == 0 {
		tags = []string{"general"}
	}

	promptMemory := strings.TrimSpace(job.Instruction)
	if promptMemory != "" {
		embed, err := s.llm.Embedding(ctx, promptMemory)
		if err == nil {
			_, addErr := s.repo.AddMemoryChunk(ctx, fmt.Sprintf("job:%d:prompt", job.ID), model.MemoryKindEpisodic, promptMemory, tags, embed)
			if addErr != nil {
				return addErr
			}
		}
	}

	responseMemory := strings.TrimSpace(response)
	if responseMemory == "" {
		return nil
	}

	embed, err := s.llm.Embedding(ctx, responseMemory)
	if err != nil {
		_, addErr := s.repo.AddMemoryChunk(ctx, fmt.Sprintf("job:%d:response", job.ID), model.MemoryKindEpisodic, responseMemory, tags, nil)
		return addErr
	}

	_, err = s.repo.AddMemoryChunk(ctx, fmt.Sprintf("job:%d:response", job.ID), model.MemoryKindEpisodic, responseMemory, tags, embed)
	return err
}

func (s *Service) memorizeSuccessfulJob(ctx context.Context, jobID int64) error {
	if s == nil || s.repo == nil || jobID <= 0 {
		return nil
	}
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return err
	}
	if details.Job.Status != model.JobStatusCompleted {
		return nil
	}
	content := buildSuccessfulJobPlaybook(details)
	if strings.TrimSpace(content) == "" {
		return nil
	}
	memoryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	contexts := contextsToMap(details.Contexts)
	tags := successfulJobPlaybookTags(details.Job, contexts)
	embed, err := s.llm.Embedding(memoryCtx, trimForBudget(content, 6000))
	if err != nil {
		embed = nil
	}
	_, err = s.repo.AddMemoryChunk(memoryCtx, fmt.Sprintf("job:%d:success_playbook", details.Job.ID), model.MemoryKindProcedural, content, tags, embed)
	return err
}

func successfulJobPlaybookTags(job model.Job, contexts map[string]string) []string {
	tags := memoryScopeTags(job, parseTagsCSV(contexts["tags"]))
	tags = appendUnique(tags,
		model.MemoryKindProcedural,
		model.MemoryTrustTagApproved,
		"success-playbook",
		"learned-skill",
		"cross-project",
		"pipeline:"+strings.ToLower(strings.TrimSpace(job.Pipeline)),
	)
	for _, token := range successfulJobKeywordTags(job.Instruction) {
		tags = appendUnique(tags, token)
	}
	if len(tags) == 0 {
		return []string{"general", "success-playbook", model.MemoryKindProcedural, model.MemoryTrustTagApproved}
	}
	return tags
}

func successfulJobKeywordTags(text string) []string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", ".", " ", ",", " ", ":", " ", ";", " ", "(", " ", ")", " ")
	normalized = replacer.Replace(normalized)
	out := []string{}
	for _, token := range strings.Fields(normalized) {
		if len(token) < 3 || len(token) > 32 {
			continue
		}
		if successfulJobStopword(token) {
			continue
		}
		out = appendUnique(out, "topic:"+token)
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

func successfulJobStopword(token string) bool {
	switch token {
	case "the", "and", "for", "with", "that", "this", "from", "into", "onto", "you", "your", "are", "can", "need", "needs", "make", "build", "create", "please", "using", "use", "app", "project":
		return true
	default:
		return false
	}
}

func buildSuccessfulJobPlaybook(details model.JobDetails) string {
	if details.Job.ID <= 0 || details.Job.Status != model.JobStatusCompleted {
		return ""
	}
	completed := []model.Step{}
	for _, step := range details.Steps {
		if step.Status != model.StepStatusCompleted {
			continue
		}
		if strings.TrimSpace(step.Output) == "" {
			continue
		}
		completed = append(completed, step)
	}
	if len(completed) == 0 {
		return ""
	}
	contextByStep := map[int64][]model.StepContext{}
	for _, ctxValue := range details.Contexts {
		contextByStep[ctxValue.StepID] = append(contextByStep[ctxValue.StepID], ctxValue)
	}
	lines := []string{
		"Successful execution playbook",
		fmt.Sprintf("job_id=%d", details.Job.ID),
		"pipeline=" + strings.TrimSpace(details.Job.Pipeline),
		"status=" + details.Job.Status,
		"",
		"Goal:",
		trimForBudget(details.Job.Instruction, 900),
		"",
		"Outcome:",
		compactPlaybookText(firstNonEmptyString(details.Job.Result, latestContextForKey(details.Contexts, "response"), latestContextForKey(details.Contexts, "assist")), 500),
		"",
		"Successful steps:",
	}
	included := 0
	for _, step := range completed {
		if actionTooNoisyForPlaybook(step.Action) {
			continue
		}
		if included >= 10 {
			lines = append(lines, "- additional successful steps omitted")
			break
		}
		action := strings.TrimSpace(step.Action)
		if action == "" {
			action = "step"
		}
		summary := compactPlaybookText(step.Output, 300)
		lines = append(lines, fmt.Sprintf("- %s: %s", action, singleLineForPlaybook(summary)))
		included++
		for _, ctxValue := range contextByStep[step.ID] {
			key := strings.TrimSpace(ctxValue.Key)
			if key == "" || !contextKeyUsefulForPlaybook(key) {
				continue
			}
			value := compactPlaybookText(ctxValue.Value, 180)
			if value == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", key, singleLineForPlaybook(value)))
		}
	}
	if included == 0 {
		for i, step := range completed {
			if i >= 5 {
				lines = append(lines, "- additional successful steps omitted")
				break
			}
			action := strings.TrimSpace(step.Action)
			if action == "" {
				action = "step"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", action, singleLineForPlaybook(compactPlaybookText(step.Output, 300))))
		}
	}
	lines = append(lines,
		"",
		"Reuse guidance:",
		"- For similar future work, retrieve this playbook by topic/tool tags and adapt the successful sequence before planning.",
		"- Prefer the recorded commands, tool choices, verification outputs, and recovery moves as experience evidence; adapt them to the current project.",
	)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func latestContextForKey(contexts []model.StepContext, key string) string {
	var latest model.StepContext
	for _, ctxValue := range contexts {
		if ctxValue.Key != key {
			continue
		}
		if ctxValue.ID >= latest.ID {
			latest = ctxValue
		}
	}
	return strings.TrimSpace(latest.Value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func singleLineForPlaybook(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func compactPlaybookText(value string, maxChars int) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	if noisyRetrievalDump(clean) {
		for _, line := range strings.Split(clean, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || noisyRetrievalDump(line) {
				continue
			}
			return trimForBudget(line, maxChars)
		}
		return "completed; noisy retrieval context omitted from reusable playbook"
	}
	return trimForBudget(clean, maxChars)
}

func noisyRetrievalDump(value string) bool {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		return false
	}
	return strings.Contains(clean, "scoped memory lookup found no matches") ||
		strings.Contains(clean, "historical memory retrieval skipped") ||
		strings.Contains(clean, "memory retrieval skipped") ||
		strings.Contains(clean, "no relevant memory needed") ||
		strings.Contains(clean, "light chat mode") ||
		strings.Contains(clean, "research chunk metadata:") ||
		strings.Contains(clean, "research memory topic=") ||
		strings.HasPrefix(clean, "source_url=") ||
		strings.HasPrefix(clean, "[1] kind=")
}

func actionTooNoisyForPlaybook(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "v3_intent_parse", "v3_capability_audit", "v3_workspace_research", "v3_memory_retrieval", "v3_external_research", "v3_memory_review":
		return true
	default:
		return false
	}
}

func contextKeyUsefulForPlaybook(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "command", "shell_command", "structured_command", "stdout", "stderr", "exit_code", "tooling", "plan", "plan_selection", "verification", "verify_action_audit", "verify_consensus", "response", "web_search", "recovery", "blocker", "failure", "objective_ledger", "structured_command_evidence":
		return true
	default:
		return false
	}
}

func contextsToMap(contexts []model.StepContext) map[string]string {
	if len(contexts) == 0 {
		return map[string]string{}
	}

	sorted := make([]model.StepContext, len(contexts))
	copy(sorted, contexts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	out := make(map[string]string, len(sorted))
	for _, ctxValue := range sorted {
		out[ctxValue.Key] = ctxValue.Value
	}

	return out
}

func parseTagsCSV(value string) []string {
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.ToLower(strings.TrimSpace(part))
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

func deriveHeuristicTags(value string, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	tokens := significantTokens(value)
	if len(tokens) == 0 {
		return nil
	}
	out := make([]string, 0, minInt(limit, len(tokens)))
	seen := map[string]struct{}{}
	add := func(tag string) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	// Promote high-signal task markers first.
	for _, marker := range []string{"file", "document", "html", "css", "javascript", "shell", "chat", "review"} {
		if len(out) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(value), marker) {
			add(marker)
		}
	}

	for _, token := range tokens {
		if len(out) >= limit {
			break
		}
		add(token)
	}
	return out
}

func resolveMemoryCandidateLimit(limit int) int {
	if limit < 1 {
		limit = 8
	}
	target := limit * 4
	if target < limit+8 {
		target = limit + 8
	}
	if target > maxMemoryRetrievalLimit {
		target = maxMemoryRetrievalLimit
	}
	return target
}

func deriveRelatedMemoryTags(scopeTags []string, matches []model.MemoryMatch, maxTags int) []string {
	if len(matches) == 0 || maxTags <= 0 {
		return nil
	}

	seeds := appendUnique(nil, scopeTags...)
	seedSet := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		seedSet[seed] = struct{}{}
	}

	scores := map[string]int{}
	for _, match := range matches {
		matchTags := appendUnique(nil, match.Tags...)
		for _, tag := range matchTags {
			if tag == "" {
				continue
			}
			if _, blocked := seedSet[tag]; blocked {
				continue
			}
			if strings.HasPrefix(tag, "project:") || strings.HasPrefix(tag, "session:") {
				continue
			}

			score := 0
			for _, seed := range seeds {
				if tagsAreRelated(tag, seed) {
					score += 2
				}
			}

			if score == 0 && len(seeds) == 0 {
				score = 1
			}
			if score > 0 {
				scores[tag] += score
			}
		}
	}

	if len(scores) == 0 {
		return nil
	}

	candidates := make([]string, 0, len(scores))
	for tag := range scores {
		candidates = append(candidates, tag)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := scores[candidates[i]]
		right := scores[candidates[j]]
		if left != right {
			return left > right
		}
		return candidates[i] < candidates[j]
	})

	if maxTags > len(candidates) {
		maxTags = len(candidates)
	}
	return candidates[:maxTags]
}

func rankMemoryOmnibusMatches(
	matches []model.MemoryMatch,
	instruction string,
	scopeTags []string,
	projectScope string,
	sessionScope string,
	limit int,
	now time.Time,
) []model.MemoryMatch {
	if len(matches) == 0 {
		return nil
	}
	if limit < 1 {
		limit = 8
	}

	merged := mergeMemoryMatches(matches, nil)
	type scoredMatch struct {
		match model.MemoryMatch
		score float64
	}

	queryTags := appendUnique(nil, scopeTags...)
	scored := make([]scoredMatch, 0, len(merged))
	for _, match := range merged {
		semanticScore := clamp01(match.Score)
		kindScore := kindAffinityScore(match.Kind, instruction)
		tagScore := memoryTagAlignmentScore(match.Tags, queryTags)
		recencyScore := memoryRecencyScore(match.CreatedAt, now)
		activityScore := memoryActivityScore(match, projectScope, sessionScope, now)

		omnibusScore := (semanticScore * 0.40) +
			(kindScore * 0.13) +
			(tagScore * 0.20) +
			(recencyScore * 0.17) +
			(activityScore * 0.10)

		scored = append(scored, scoredMatch{
			match: match,
			score: omnibusScore,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		diff := scored[i].score - scored[j].score
		if diff > 0.000001 {
			return true
		}
		if diff < -0.000001 {
			return false
		}
		if !scored[i].match.CreatedAt.Equal(scored[j].match.CreatedAt) {
			return scored[i].match.CreatedAt.After(scored[j].match.CreatedAt)
		}
		if scored[i].match.Score != scored[j].match.Score {
			return scored[i].match.Score > scored[j].match.Score
		}
		return scored[i].match.ID > scored[j].match.ID
	})

	if limit > len(scored) {
		limit = len(scored)
	}
	out := make([]model.MemoryMatch, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scored[i].match)
	}
	return out
}

func kindAffinityScore(kind string, instruction string) float64 {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	lower := strings.ToLower(strings.TrimSpace(instruction))
	if normalizedKind == "" {
		return 0.2
	}

	score := 0.45
	switch normalizedKind {
	case model.MemoryKindInstruction, model.MemoryKindProcedural:
		if strings.Contains(lower, "how") ||
			strings.Contains(lower, "build") ||
			strings.Contains(lower, "implement") ||
			strings.Contains(lower, "fix") ||
			strings.Contains(lower, "debug") ||
			strings.Contains(lower, "set up") ||
			strings.Contains(lower, "setup") {
			score += 0.35
		}
	case model.MemoryKindPreference:
		if strings.Contains(lower, "prefer") ||
			strings.Contains(lower, "preference") ||
			strings.Contains(lower, "style") ||
			strings.Contains(lower, "tone") ||
			strings.Contains(lower, "always") ||
			strings.Contains(lower, "never") {
			score += 0.45
		}
	case model.MemoryKindReference:
		if strings.Contains(lower, "reference") ||
			strings.Contains(lower, "docs") ||
			strings.Contains(lower, "documentation") ||
			strings.Contains(lower, "api") ||
			strings.Contains(lower, "version") ||
			strings.Contains(lower, "spec") {
			score += 0.35
		}
	case model.MemoryKindEpisodic:
		if memoryLookbackPattern.MatchString(lower) ||
			strings.Contains(lower, "what did") ||
			strings.Contains(lower, "we said") ||
			strings.Contains(lower, "earlier") ||
			strings.Contains(lower, "recent") {
			score += 0.50
		}
	}

	if strings.Contains(lower, "right now") || strings.Contains(lower, "currently") {
		if normalizedKind == model.MemoryKindEpisodic {
			score += 0.10
		}
	}
	return clamp01(score)
}
