package omni

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/websearch"
	"strings"
	"time"
)

func BuildWebResearchQueries(input string, plan ContextToolPlan, limit int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if limit <= 0 {
		limit = 4
	}
	lower := strings.ToLower(input)
	terms := importantSearchTerms(input)
	joinedTerms := strings.Join(terms, " ")
	queries := []string{}
	add := func(query string) {
		query = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
		if query != "" {
			queries = append(queries, query)
		}
	}
	switch {
	case strings.Contains(lower, "weather"):
		location := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(joinedTerms, "weather", ""), "current", ""))
		add(strings.TrimSpace(location + " weather forecast current conditions"))
		add(strings.TrimSpace(location + " hourly weather today"))
	case strings.Contains(lower, "news") || strings.Contains(lower, "current events"):
		add(joinedTerms + " latest news")
		add(joinedTerms + " breaking news today")
	case plan.NeedsDocuments || technicalSearchLooksLikely(lower):
		add(joinedTerms + " official documentation")
		add(joinedTerms + " getting started guide")
		add(joinedTerms + " examples best practices")
	default:
		add(joinedTerms + " official source")
		add(joinedTerms + " latest reference")
	}
	add(input)
	return limitStrings(dedupeStrings(queries), limit)
}

func importantSearchTerms(input string) []string {
	words := strings.Fields(webDocNonWordQuery.ReplaceAllString(input, " "))
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "app": true, "application": true, "as": true, "be": true, "build": true,
		"can": true, "create": true, "for": true, "how": true, "i": true, "in": true, "is": true, "it": true,
		"make": true, "me": true, "of": true, "on": true, "please": true, "should": true, "the": true, "to": true,
		"use": true, "using": true, "with": true, "what": true, "right": true, "now": true,
	}
	terms := []string{}
	for _, word := range words {
		clean := strings.Trim(word, " .,;:!?\"'`()[]{}")
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)
		if stop[lower] {
			continue
		}
		terms = append(terms, clean)
		if len(terms) >= 8 {
			break
		}
	}
	if len(terms) == 0 {
		return []string{input}
	}
	return terms
}

func technicalSearchLooksLikely(lower string) bool {
	for _, needle := range []string{"api", "sdk", "react", "vite", "node", "javascript", "typescript", "rust", "go", "golang", "zig", "php", "docker", "postgres", "pgsql", "cli", "library", "framework"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func dedupeWebSearchResults(results []websearch.Result) []websearch.Result {
	seen := map[string]struct{}{}
	out := make([]websearch.Result, 0, len(results))
	for _, result := range results {
		key := strings.TrimSpace(result.URL)
		if key == "" {
			key = strings.TrimSpace(result.Title + "\n" + result.Snippet)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, result)
	}
	return out
}

type staticWebSearchResults struct {
	results []websearch.Result
}

func (s staticWebSearchResults) SearchAll(ctx context.Context, query string) ([]websearch.Result, error) {
	return s.results, nil
}

func webSearchTimelineDetails(query string, results []websearch.Result) map[string]string {
	return map[string]string{
		"query":       strings.TrimSpace(query),
		"results":     fmt.Sprintf("%d", len(results)),
		"providers":   strings.Join(webSearchProviders(results), ","),
		"search_urls": strings.Join(webSearchSearchURLs(results, 5), "\n"),
		"result_urls": strings.Join(webSearchResultURLs(results, 5), "\n"),
	}
}

func webSearchProviders(results []websearch.Result) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		provider := strings.TrimSpace(result.Provider)
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true
		out = append(out, provider)
	}
	return out
}

func webSearchSearchURLs(results []websearch.Result, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		url := strings.TrimSpace(result.SearchURL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, url)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func webSearchResultURLs(results []websearch.Result, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, result := range results {
		url := strings.TrimSpace(result.URL)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, url)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (a *App) handleManagerTurn(session *Session, objective string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{a.newEvent("manager_started", "Manager-worker job started", map[string]string{
		"permission_mode": string(session.Permission),
	})}

	_ = a.runLogger.Log("manager", "turn_started", map[string]interface{}{
		"objective":       objective,
		"permission_mode": session.Permission,
		"workspace":       session.WorkspacePath,
	})

	execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	result, execErr := ExecuteManagerWorkerJob(execCtx, session, objective, session.Permission, a.in, a.out, a.ollama, a.nextEventID, a.runLogger)
	cancel()
	events = append(events, result.Events...)

	assistantResponse := result.Summary
	if execErr != nil {
		assistantResponse = fmt.Sprintf("Manager execution failed: %v", execErr)
		events = append(events, a.newEvent("manager_failed", "Manager-worker job terminated with error", map[string]string{"error": execErr.Error()}))
	} else {
		events = append(events, a.newEvent("manager_completed", "Manager-worker job completed", map[string]string{
			"done":     fmt.Sprintf("%t", result.Done),
			"tasks":    fmt.Sprintf("%d", len(result.Tasks)),
			"workers":  fmt.Sprintf("%d", len(result.Workers)),
			"executed": fmt.Sprintf("%d", result.Executed),
			"blocked":  fmt.Sprintf("%d", result.Blocked),
			"failed":   fmt.Sprintf("%d", result.Failed),
		}))
	}
	assistantResponse = a.reviewFinalResponse(context.Background(), "/manage "+objective, assistantResponse, []string{result.Summary}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})

	turn := Turn{
		ID:                   turnID,
		UserInput:            "/manage " + objective,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"manager_worker_job"},
		Response:             assistantResponse,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, assistantResponse, nil
}

func (a *App) handleResearchTurn(session *Session, query string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	events := []Event{a.newEvent("research_started", "Web research memory job started", map[string]string{"query": query})}
	if a.web == nil {
		events = append(events, a.newEvent("research_blocked", "Web search service is unavailable", nil))
		return Turn{}, "", fmt.Errorf("web search service is unavailable")
	}
	if a.memory == nil {
		events = append(events, a.newEvent("research_blocked", "Postgres memory is not configured", map[string]string{
			"hint": "set --memory-database-url or OMNI_MEMORY_DATABASE_URL",
		}))
		turn := Turn{
			ID:                   turnID,
			UserInput:            "/research " + query,
			IntentClassification: IntentExecution,
			Confidence:           1.0,
			ReasonCodes:          []string{"web_research_memory"},
			Events:               events,
			CreatedAt:            nowUTC(),
		}
		response := "Research blocked: Postgres memory is not configured. Set --memory-database-url or OMNI_MEMORY_DATABASE_URL."
		response = a.reviewFinalResponse(context.Background(), turn.UserInput, response, []string{"Postgres memory is not configured"}, func(eventType, summary string, details map[string]string) {
			turn.Events = append(turn.Events, a.newEvent(eventType, summary, details))
		})
		turn.Response = response
		return turn, turn.Response, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	events = append(events, a.newEvent("web_search_started", "Web search started", map[string]string{
		"query": query,
		"role":  "web_research_specialist",
	}))
	result, err := ResearchWebToMemory(ctx, query, a.web, a.memory, WebResearchMemoryConfig{
		AgentID: "omni_research_manager",
		Tags:    researchTagsFromQuery(query),
	})
	if err != nil {
		events = append(events, a.newEvent("research_failed", "Web research memory job failed", map[string]string{"error": err.Error()}))
		return Turn{}, "", err
	}
	events = append(events, a.newEvent("web_search_completed", "Web search completed", webSearchTimelineDetails(query, result.Results)))
	events = append(events, a.newEvent("research_completed", "Web research stored in Postgres memory", map[string]string{
		"query":        query,
		"results":      fmt.Sprintf("%d", len(result.Results)),
		"stored":       fmt.Sprintf("%d", result.StoredCount),
		"stored_agent": "omni_research_manager",
		"result_urls":  strings.Join(webSearchResultURLs(result.Results, 5), "\n"),
	}))
	response := fmt.Sprintf("Stored %d web research memory chunk(s) from %d search result(s) for: %s", result.StoredCount, len(result.Results), query)
	response = a.reviewFinalResponse(context.Background(), "/research "+query, response, []string{
		fmt.Sprintf("stored=%d results=%d", result.StoredCount, len(result.Results)),
	}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})
	turn := Turn{
		ID:                   turnID,
		UserInput:            "/research " + query,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"web_research_memory"},
		Response:             response,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, response, nil
}

func researchCommandQuery(input string) (string, bool) {
	cmd, ok := parseChatSlashCommand(input)
	if !ok || cmd.Kind != chatSlashResearch {
		return "", false
	}
	return cmd.Args, cmd.Args != ""
}

func managerObjective(input string) (string, bool) {
	cmd, ok := parseChatSlashCommand(input)
	if !ok || cmd.Kind != chatSlashManage {
		return "", false
	}
	return cmd.Args, cmd.Args != ""
}

func microQueueObjective(input string) (string, bool) {
	cmd, ok := parseChatSlashCommand(input)
	if !ok || cmd.Kind != chatSlashMicro {
		return "", false
	}
	return cmd.Args, cmd.Args != ""
}

func researchTagsFromQuery(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	tags := []string{"web-research"}
	for _, part := range parts {
		clean := strings.Trim(part, ".,;:!?()[]{}\"'")
		if len(clean) >= 4 {
			tags = append(tags, clean)
		}
		if len(tags) >= 8 {
			break
		}
	}
	return cleanMemoryTags(tags)
}

func formatMicroQueueResponse(result MicroJobQueueResult, stdout, stderr, errText string) string {
	lines := []string{
		result.Summary,
		fmt.Sprintf("Jobs: %d", len(result.Jobs)),
		fmt.Sprintf("Completed: %d", len(result.Results)),
		fmt.Sprintf("Done: %t", result.Done),
	}
	if strings.TrimSpace(result.ProjectProfile.Summary) != "" {
		lines = append(lines, "Profile: "+result.ProjectProfile.Summary)
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "Stdout: "+truncateOutput(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "Stderr: "+truncateOutput(stderr))
	}
	if strings.TrimSpace(errText) != "" {
		lines = append(lines, "Error: "+errText)
	}
	return strings.Join(lines, "\n")
}
