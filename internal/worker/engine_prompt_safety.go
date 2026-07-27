package worker

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/websearch"
	"sort"
	"strconv"
	"strings"
)

func sanitizePromptBlockBody(value string) string {
	body := strings.TrimSpace(value)
	if body == "" {
		return "(empty)"
	}
	body = strings.ReplaceAll(body, "\x00", "")
	body = strings.ReplaceAll(body, "<", "&lt;")
	body = strings.ReplaceAll(body, ">", "&gt;")
	return body
}

func normalizePromptBlockName(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "SECTION"
	}

	var b strings.Builder
	lastUnderscore := false
	for _, ch := range value {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			lastUnderscore = false
			continue
		}
		if lastUnderscore {
			continue
		}
		b.WriteByte('_')
		lastUnderscore = true
	}

	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "SECTION"
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
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

func (s *Service) prepareTournamentContext(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	sourceKey string,
	value string,
	budget int,
) (string, error) {
	value = trimForBudget(value, budget)
	if strings.TrimSpace(value) == "" {
		return value, nil
	}
	if !s.tournament.Enabled {
		return value, nil
	}
	if len(value) <= s.tournament.ChunkChars {
		return value, nil
	}

	report, summary, err := s.tournamentSummarizeContext(ctx, stepID, modelName, goal, sourceKey, value)
	if err != nil {
		s.emitStepEvent(stepID, "tournament_failed", "source="+safeLine(sourceKey, "unknown"))
		return "", fmt.Errorf("tournament context summarization failed for %q: %w", sourceKey, err)
	}
	if strings.TrimSpace(summary) == "" {
		return "", fmt.Errorf("tournament context summarization returned empty output for %q", sourceKey)
	}

	s.emitStepEvent(
		stepID,
		"tournament_ready",
		fmt.Sprintf(
			"source=%s raw_chars=%d chunks=%d selected=%d verified=%d rounds=%d output_chars=%d",
			report.Source,
			report.RawChars,
			report.LeafChunks,
			report.SelectedLeaves,
			report.VerifiedLeaves,
			report.Rounds,
			report.OutputChars,
		),
	)
	s.emitStepContext(
		stepID,
		sourceKey+"_tournament",
		fmt.Sprintf(
			"raw_chars=%d leaf_chunks=%d selected_leaves=%d verified_leaves=%d rounds=%d output_chars=%d",
			report.RawChars,
			report.LeafChunks,
			report.SelectedLeaves,
			report.VerifiedLeaves,
			report.Rounds,
			report.OutputChars,
		),
	)
	return trimForBudget(summary, budget), nil
}

type tournamentContextRequest struct {
	SourceKey string
	Value     string
	Budget    int
}

func (s *Service) prepareTournamentContexts(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	requests []tournamentContextRequest,
) (map[string]string, error) {
	prepared := make(map[string]string, len(requests))
	for _, request := range requests {
		value, err := s.prepareTournamentContext(ctx, stepID, modelName, goal, request.SourceKey, request.Value, request.Budget)
		if err != nil {
			return nil, err
		}
		prepared[request.SourceKey] = value
	}
	return prepared, nil
}

func (s *Service) tournamentSummarizeContext(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	sourceKey string,
	value string,
) (tournamentReport, string, error) {
	report := tournamentReport{
		Source:   sourceKey,
		RawChars: len(strings.TrimSpace(value)),
	}
	chunks := splitTournamentChunks(value, s.tournament.ChunkChars)
	if len(chunks) == 0 {
		return report, "", nil
	}
	report.LeafChunks = len(chunks)

	leaves := make([]tournamentLeafSummary, 0, len(chunks))
	for idx, chunk := range chunks {
		leaf, err := s.summarizeTournamentLeaf(ctx, stepID, modelName, goal, sourceKey, idx, chunk)
		if err != nil {
			return report, "", err
		}
		leaves = append(leaves, leaf)
	}

	selected := make([]tournamentLeafSummary, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.Relevant {
			selected = append(selected, leaf)
		}
	}
	if len(selected) == 0 {
		selected = topTournamentLeafsByConfidence(leaves, minInt(4, len(leaves)))
	}

	if s.tournament.VerifySupport {
		verified := make([]tournamentLeafSummary, 0, len(selected))
		for _, leaf := range selected {
			updated, keep, err := s.verifyTournamentLeaf(ctx, stepID, modelName, goal, sourceKey, leaf)
			if err != nil {
				verified = append(verified, leaf)
				continue
			}
			if !keep {
				continue
			}
			verified = append(verified, updated)
		}
		if len(verified) > 0 {
			selected = verified
		}
	}

	report.SelectedLeaves = len(selected)
	for _, leaf := range selected {
		if leaf.Verified {
			report.VerifiedLeaves++
		}
	}
	if len(selected) == 0 {
		return report, "", nil
	}

	items := make([]string, 0, len(selected))
	for _, leaf := range selected {
		conf := leaf.Confidence
		if conf < 0 {
			conf = 0
		}
		items = append(items, fmt.Sprintf("[chunk %d conf=%d] %s", leaf.Index+1, conf, strings.TrimSpace(leaf.Summary)))
	}

	current := strings.Join(items, "\n")
	rounds := 0
	for rounds < s.tournament.MaxRounds {
		if len(strings.TrimSpace(current)) <= s.tournament.SummaryChars {
			break
		}
		rounds++
		next, err := s.tournamentRoundSummarize(ctx, stepID, modelName, goal, sourceKey, current, rounds)
		if err != nil {
			break
		}
		if strings.TrimSpace(next) == "" || strings.TrimSpace(next) == strings.TrimSpace(current) {
			break
		}
		current = next
	}
	if rounds == 0 {
		rounds = 1
	}
	report.Rounds = rounds
	current = trimForBudget(current, s.tournament.SummaryChars)
	report.OutputChars = len(strings.TrimSpace(current))
	return report, current, nil
}

func (s *Service) summarizeTournamentLeaf(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	sourceKey string,
	index int,
	chunk string,
) (tournamentLeafSummary, error) {
	prompt := strings.Join([]string{
		"You are a precision extractor in a hierarchical tournament summarization pipeline.",
		antiRoleplayInstruction(),
		"Determine whether CHUNK is relevant to GOAL and summarize only supported facts.",
		"Return EXACT format:",
		"RELEVANT: yes|no",
		"CONFIDENCE: 0-100",
		"SUMMARY: one concise paragraph",
		"EVIDENCE: short quote or concrete anchor from CHUNK",
		"GOAL:",
		strings.TrimSpace(goal),
		"SOURCE:",
		sourceKey,
		"CHUNK:",
		trimForBudget(chunk, s.tournament.ChunkChars),
	}, "\n\n")
	raw, err := s.llmGenerateWithTrace(
		ctx,
		stepID,
		fmt.Sprintf("tournament_leaf_summary_%s_chunk_%d", sourceKey, index+1),
		modelName,
		prompt,
	)
	if err != nil {
		return tournamentLeafSummary{}, err
	}
	relevant := strings.EqualFold(parseTournamentField(raw, "RELEVANT"), "yes")
	confidence := parseTournamentConfidence(raw)
	summary := strings.TrimSpace(parseTournamentField(raw, "SUMMARY"))
	if summary == "" {
		summary = trimForBudget(strings.TrimSpace(chunk), minInt(s.tournament.SummaryChars/2, 280))
	}
	return tournamentLeafSummary{
		Index:      index,
		Relevant:   relevant,
		Confidence: confidence,
		Summary:    summary,
		Chunk:      chunk,
		Verified:   false,
		Supported:  "",
	}, nil
}

func (s *Service) verifyTournamentLeaf(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	sourceKey string,
	leaf tournamentLeafSummary,
) (tournamentLeafSummary, bool, error) {
	prompt := strings.Join([]string{
		antiRoleplayInstruction(),
		"Validate CLAIMED_SUMMARY against ORIGINAL_CHUNK.",
		"If unsupported, provide a corrected summary with only supported facts.",
		"Return EXACT format:",
		"SUPPORTED: yes|partial|no",
		"CORRECTED_SUMMARY: one concise paragraph",
		"RATIONALE: one sentence",
		"GOAL:",
		strings.TrimSpace(goal),
		"SOURCE:",
		sourceKey,
		"CLAIMED_SUMMARY:",
		strings.TrimSpace(leaf.Summary),
		"ORIGINAL_CHUNK:",
		trimForBudget(leaf.Chunk, s.tournament.ChunkChars),
	}, "\n\n")
	raw, err := s.llmGenerateWithTrace(
		ctx,
		stepID,
		fmt.Sprintf("tournament_leaf_verify_%s_chunk_%d", sourceKey, leaf.Index+1),
		modelName,
		prompt,
	)
	if err != nil {
		return leaf, true, err
	}
	supported := strings.ToLower(strings.TrimSpace(parseTournamentField(raw, "SUPPORTED")))
	corrected := strings.TrimSpace(parseTournamentField(raw, "CORRECTED_SUMMARY"))
	updated := leaf
	updated.Verified = true
	updated.Supported = supported
	if corrected != "" {
		updated.Summary = corrected
	}
	switch supported {
	case "yes", "partial":
		return updated, true, nil
	case "no":
		return updated, false, nil
	default:
		return updated, true, nil
	}
}

func (s *Service) tournamentRoundSummarize(
	ctx context.Context,
	stepID int64,
	modelName string,
	goal string,
	sourceKey string,
	value string,
	round int,
) (string, error) {
	groupSize := 4
	lines := splitTournamentChunks(value, s.tournament.ChunkChars)
	if len(lines) == 0 {
		return "", nil
	}
	if len(lines) == 1 {
		prompt := strings.Join([]string{
			antiRoleplayInstruction(),
			"Compress this evidence summary while preserving factual details.",
			fmt.Sprintf("Keep output under %d characters.", s.tournament.SummaryChars),
			"GOAL:",
			strings.TrimSpace(goal),
			"SOURCE:",
			sourceKey,
			"SUMMARY_INPUT:",
			strings.TrimSpace(lines[0]),
		}, "\n\n")
		return s.llmGenerateWithTrace(
			ctx,
			stepID,
			fmt.Sprintf("tournament_round_%d_single_%s", round, sourceKey),
			modelName,
			prompt,
		)
	}

	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i += groupSize {
		end := i + groupSize
		if end > len(lines) {
			end = len(lines)
		}
		group := strings.Join(lines[i:end], "\n")
		prompt := strings.Join([]string{
			antiRoleplayInstruction(),
			"Merge these mini summaries into one tighter summary with no speculation.",
			fmt.Sprintf("Keep output under %d characters.", minInt(s.tournament.SummaryChars, 650)),
			"GOAL:",
			strings.TrimSpace(goal),
			"SOURCE:",
			sourceKey,
			"MINI_SUMMARIES:",
			group,
		}, "\n\n")
		merged, err := s.llmGenerateWithTrace(
			ctx,
			stepID,
			fmt.Sprintf("tournament_round_%d_group_%s_%d", round, sourceKey, (i/groupSize)+1),
			modelName,
			prompt,
		)
		if err != nil {
			return "", err
		}
		merged = strings.TrimSpace(merged)
		if merged == "" {
			merged = trimForBudget(group, minInt(s.tournament.SummaryChars, 650))
		}
		out = append(out, merged)
	}
	return strings.Join(out, "\n"), nil
}

func splitTournamentChunks(value string, maxChars int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if maxChars < 256 {
		maxChars = 256
	}

	parts := strings.Split(value, "\n")
	chunks := make([]string, 0, len(parts)/3+1)
	var b strings.Builder
	appendChunk := func() {
		chunk := strings.TrimSpace(b.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		b.Reset()
	}

	for _, line := range parts {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if len(clean) > maxChars {
			if b.Len() > 0 {
				appendChunk()
			}
			runes := []rune(clean)
			for start := 0; start < len(runes); start += maxChars {
				end := start + maxChars
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[start:end]))
			}
			continue
		}
		if b.Len() == 0 {
			b.WriteString(clean)
			continue
		}
		if b.Len()+1+len(clean) > maxChars {
			appendChunk()
			b.WriteString(clean)
			continue
		}
		b.WriteString("\n")
		b.WriteString(clean)
	}
	if b.Len() > 0 {
		appendChunk()
	}
	return chunks
}

func parseTournamentField(raw, key string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	prefix := strings.ToUpper(strings.TrimSpace(key)) + ":"
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}

func parseTournamentConfidence(raw string) int {
	field := strings.TrimSpace(parseTournamentField(raw, "CONFIDENCE"))
	if field == "" {
		return 50
	}
	field = strings.TrimSuffix(field, "%")
	parsed, err := strconv.Atoi(strings.TrimSpace(field))
	if err != nil {
		if label, ok := parseTournamentConfidenceLabel(field); ok {
			return label
		}
		return 50
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 100 {
		return 100
	}
	return parsed
}

func parseTournamentConfidenceLabel(raw string) (int, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "very high", "high confidence", "strong", "certain":
		return 90, true
	case "high":
		return 80, true
	case "medium-high", "med-high":
		return 70, true
	case "medium", "moderate":
		return 55, true
	case "medium-low", "med-low":
		return 40, true
	case "low":
		return 25, true
	case "very low", "minimal":
		return 10, true
	default:
		return 0, false
	}
}

func topTournamentLeafsByConfidence(items []tournamentLeafSummary, limit int) []tournamentLeafSummary {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	sorted := append([]tournamentLeafSummary{}, items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Confidence == sorted[j].Confidence {
			return sorted[i].Index < sorted[j].Index
		}
		return sorted[i].Confidence > sorted[j].Confidence
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Index < sorted[j].Index
	})
	return sorted
}

func (s *Service) runWebSearchStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	mode := webSearchMode(claim.Job.Metadata)
	planNeedsExternal, planDecided := planNeedsExternalInfo(contexts["plan"])
	persistent := persistentExecutionEnabled(claim.Job)
	feedback := strings.TrimSpace(strings.Join([]string{
		contexts["user_feedback"],
		metadataString(claim.Job.Metadata, "replan_feedback"),
	}, "\n"))
	forceFreshExternal := shouldForceFreshWebSearch(claim.Job.Instruction, feedback)
	timeSensitive := isTimeSensitiveInstruction(claim.Job.Instruction) || forceFreshExternal
	localClockOnly := isLocalClockOnlyInstruction(claim.Job.Instruction)
	if mode == "off" && forceFreshExternal {
		mode = "force"
		s.emitStepEvent(claim.Step.ID, "web_search_override", "reason=explicit_user_freshness_or_web_request")
	}
	s.emitStepEvent(claim.Step.ID, "web_search_begin", fmt.Sprintf("mode=%s time_sensitive=%t", mode, timeSensitive))
	if mode == "off" {
		output := "web search skipped: metadata mode=off"
		s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=mode_off")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
	}

	if mode == "auto" {
		if localClockOnly && !forceFreshExternal {
			output := "web search skipped: local clock/date query should use host time context"
			s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=local_clock_query")
			return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
		}
		if !forceFreshExternal && !timeSensitive && planDecided && !planNeedsExternal {
			output := "web search skipped: plan says no external info needed"
			s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=plan_no_external")
			return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
		}
		if !forceFreshExternal && !timeSensitive && (!planDecided || !planNeedsExternal) {
			if !shouldRunWebSearch(strings.TrimSpace(strings.Join([]string{claim.Job.Instruction, feedback}, "\n"))) {
				output := "web search skipped: heuristic not triggered"
				s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=heuristic_not_triggered")
				return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
			}
		}
	}
	if mode == "auto" && s.cognition.StopOnSufficientContext && !timeSensitive && !forceFreshExternal {
		if !planDecided || !planNeedsExternal {
			if hasSufficientRetrievedContext(contexts["retrieval"], s.cognition.SufficientContextChars) {
				output := "web search skipped: sufficient memory context already available"
				s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=sufficient_memory_context")
				return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
			}
		}
	}

	if s.webSearch == nil {
		output := "web search unavailable: service disabled"
		if planNeedsExternal && !persistent {
			question := "Planner requires fresh external info, but web search is disabled. Enable web search, provide manual references, or submit feedback to continue without it."
			s.emitStepEvent(claim.Step.ID, "web_search_waiting_input", "reason=service_disabled")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"web_search": output,
			})
		}
		s.emitStepEvent(claim.Step.ID, "web_search_skipped", "reason=service_disabled")
		return s.repo.CompleteStep(ctx, claim.Step.ID, output, "web_search", output)
	}

	query := metadataString(claim.Job.Metadata, "search_query")
	if strings.TrimSpace(query) == "" {
		query = s.deriveSearchQuery(ctx, claim.Step.ID, claim.Job, contexts)
	}
	if strings.TrimSpace(query) == "" {
		query = claim.Job.Instruction
	}
	s.emitStepStream(claim.Step.ID, "stdout", "web search query: "+strings.TrimSpace(query))

	report, err := s.webSearch.SearchAllDetailed(ctx, query)
	emitWebSearchProviderDiagnostics(s, claim.Step.ID, report.Diagnostics)
	if err != nil {
		s.emitStepStream(claim.Step.ID, "stderr", "web search error: "+err.Error())
		output := fmt.Sprintf("web search failed for query %q: %v", strings.TrimSpace(query), err)
		if planNeedsExternal && !persistent {
			question := "Web search failed but fresh context is required. Provide a better query/source hints (or disable web requirement) and submit feedback."
			s.emitStepEvent(claim.Step.ID, "web_search_waiting_input", "reason=search_failed")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, output, question, map[string]string{
				"web_search":   output,
				"search_query": strings.TrimSpace(query),
			})
		}
		s.emitStepEvent(claim.Step.ID, "web_search_failed", "reason=provider_failure")
		return fmt.Errorf("%s", output)
	}
	results := report.Results

	if persisted, persistErr := s.persistWebSearchResults(ctx, claim.Job, query, results, contexts); persistErr != nil {
		s.logger.Printf("job=%d web search memory persist warning: %v", claim.Job.ID, persistErr)
		s.emitStepEvent(claim.Step.ID, "web_search_memory_warning", "reason="+safeLine(persistErr.Error(), "persist_failed"))
	} else if persisted > 0 {
		s.emitStepEvent(claim.Step.ID, "web_search_memory_persisted", fmt.Sprintf("chunks=%d", persisted))
	}

	webContext := websearch.BuildContext(results, s.contextBudget)
	webContext = trimForBudget(webContext, s.contextBudget)
	if strings.TrimSpace(webContext) == "" {
		webContext = "web search returned no usable content"
		if planNeedsExternal && !persistent {
			question := "No usable web results were extracted. Provide source links or a tighter query and submit feedback."
			s.emitStepEvent(claim.Step.ID, "web_search_waiting_input", "reason=no_usable_results")
			return s.repo.PauseStepForInput(ctx, claim.Step.ID, webContext, question, map[string]string{
				"web_search":   webContext,
				"search_query": strings.TrimSpace(query),
			})
		}
	}
	s.emitStepStream(claim.Step.ID, "stdout", fmt.Sprintf("web search context chars=%d", len(webContext)))
	s.emitStepEvent(claim.Step.ID, "web_search_ready", fmt.Sprintf("context_chars=%d", len(webContext)))

	return s.repo.CompleteStep(ctx, claim.Step.ID, webContext, "web_search", webContext)
}

func emitWebSearchProviderDiagnostics(s *Service, stepID int64, diagnostics []websearch.ProviderDiagnostic) {
	if s == nil || len(diagnostics) == 0 {
		return
	}
	for _, diagnostic := range diagnostics {
		provider := strings.TrimSpace(diagnostic.Provider)
		if provider == "" {
			provider = "unknown"
		}
		if strings.TrimSpace(diagnostic.Error) != "" {
			s.emitStepEvent(stepID, "web_search_provider_failed", fmt.Sprintf("provider=%s error=%s", provider, safeLine(diagnostic.Error, "failed")))
			continue
		}
		if diagnostic.Succeeded {
			s.emitStepEvent(stepID, "web_search_provider_succeeded", fmt.Sprintf("provider=%s results=%d", provider, diagnostic.ResultCount))
		}
	}
}
