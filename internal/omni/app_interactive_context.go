package omni

import (
	"bufio"
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/websearch"
	"io"
	"strings"
	"sync"
	"time"
)

func (a *App) startTurnActivity(session *Session) *activityIndicator {
	if session.Permission != PermissionFull || !isInteractiveWriter(a.out) {
		return &activityIndicator{}
	}
	return startActivityIndicator(a.out, "working")
}

func (a *App) startEscapeInterrupt(ctx context.Context, cancel context.CancelFunc, activity *activityIndicator, emitEvent func(string, string, map[string]string)) func() {
	if a.terminalIn == nil || cancel == nil || !isInteractive(a.terminalIn) || !isInteractiveWriter(a.out) {
		return func() {}
	}
	fd := int(a.terminalIn.Fd())
	restore, err := enableTerminalCbreak(fd)
	if err != nil {
		return func() {}
	}
	done := make(chan struct{})
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		defer restore()
		buffer := []byte{0}
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			default:
			}
			ready, err := pollTerminalInput(fd, 100*time.Millisecond)
			if err != nil {
				return
			}
			if !ready {
				continue
			}
			n, err := readTerminalByte(fd, buffer)
			if err != nil || n == 0 {
				continue
			}
			if buffer[0] != 0x1b {
				continue
			}
			cancel()
			if activity != nil {
				activity.Pause()
			}
			if emitEvent != nil {
				emitEvent("user_interrupt_requested", "User pressed Esc to interrupt the active turn", map[string]string{
					"input": "esc",
				})
			}
			if activity != nil {
				activity.Pause()
			}
			return
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

func readLineFromReader(ctx context.Context, reader io.Reader) (string, error) {
	type stringReader interface {
		ReadString(delim byte) (string, error)
	}
	if sr, ok := reader.(stringReader); ok {
		return readLineWithContext(ctx, sr)
	}
	return readLineWithContext(ctx, bufio.NewReader(reader))
}

func readLineWithContext(ctx context.Context, reader interface {
	ReadString(delim byte) (string, error)
}) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		done <- result{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-done:
		if res.err != nil && res.err != io.EOF {
			return "", res.err
		}
		return res.line, nil
	}
}

func (a *App) ollamaModelName() string {
	if a.ollama == nil {
		return "disabled"
	}
	return a.ollama.Model
}

func (a *App) structuredPlannerClient() CommandDecisionClient {
	if a.plannerClient != nil {
		return a.plannerClient
	}
	if a.planner != nil {
		return a.planner
	}
	if a.ollama != nil {
		return a.ollama
	}
	return nil
}

func (a *App) planContextForTurn(ctx context.Context, input string) (ContextToolPlan, []Event) {
	plan, err := PlanContextTools(ctx, a.planner, input)
	plan = AugmentContextToolPlan(input, plan)
	events := []Event{a.newEvent("context_plan_created", "Context tool plan created", map[string]string{
		"tools":  strings.Join(plan.Tools, ","),
		"reason": plan.Reason,
	})}
	if err != nil {
		events = append(events, a.newEvent("context_plan_failed", "Context tool planner fell back to default", map[string]string{"error": err.Error()}))
	}
	return plan, events
}

type interactivePrepContext struct {
	Plan            ContextToolPlan
	Memory          interactiveMemoryContext
	SessionMemories []SessionMemory
	Bundle          PrepContextBundle
	Validation      PrepValidation
}

func (a *App) prepareInteractiveTurnContext(ctx context.Context, input, activeDirectory string, emitEvent func(string, string, map[string]string)) interactivePrepContext {
	prep := interactivePrepContext{}
	emitEvent("prep_started", "Preparing compact task context", map[string]string{
		"active_directory": activeDirectory,
	})
	plan, planEvents := a.planContextForTurn(ctx, input)
	prep.Plan = plan
	for _, event := range planEvents {
		emitEvent(event.Type, event.Summary, event.Details)
	}
	var route TaskRoute
	if routeMemory, preparedRoute, ok := a.prepareCodebaseRouteBrief(ctx, activeDirectory, input, emitEvent); ok {
		route = preparedRoute
		prep.SessionMemories = append(prep.SessionMemories, routeMemory)
	}
	memoryCtx := a.loadInteractiveMemoryContext(ctx, input, activeDirectory, emitEvent)
	prep.Memory = memoryCtx
	prep.SessionMemories = append(prep.SessionMemories, memoryCtx.Memories...)
	if docMemory, ok := a.prepareDocumentationBrief(ctx, input, memoryCtx.Tags, plan, emitEvent); ok {
		prep.SessionMemories = append(prep.SessionMemories, docMemory)
	}
	if webMemory, ok := a.prepareWebResearchBrief(ctx, input, plan, emitEvent); ok {
		prep.SessionMemories = append(prep.SessionMemories, webMemory)
	}
	survey := BuildWorksiteSurvey(activeDirectory)
	prep.Bundle = CompactPrepContextBundle(NewPrepContextBundle("interactive-turn", activeDirectory, survey, plan, route, prep.SessionMemories), defaultPrepContextBudgetLimit)
	prep.Validation = ValidatePrepContextBundle(prep.Bundle, plan)
	emitEvent("prep_context_built", "Preparation context bundle built", map[string]string{
		"briefs":       fmt.Sprintf("%d", len(allPrepBriefs(prep.Bundle))),
		"evidence":     fmt.Sprintf("%d", len(prep.Bundle.Evidence)),
		"budget_used":  fmt.Sprintf("%d", prep.Bundle.ContextBudgetUsed),
		"budget_limit": fmt.Sprintf("%d", prep.Bundle.ContextBudgetLimit),
		"compressed":   fmt.Sprintf("%t", prep.Bundle.Compressed),
	})
	validationType := "prep_context_validated"
	validationSummary := "Preparation context bundle validated"
	if !prep.Validation.Valid {
		validationType = "prep_context_validation_failed"
		validationSummary = "Preparation context bundle failed validation"
	}
	emitEvent(validationType, validationSummary, map[string]string{
		"valid":    fmt.Sprintf("%t", prep.Validation.Valid),
		"failures": strings.Join(prep.Validation.Failures, ","),
		"warnings": strings.Join(prep.Validation.Warnings, ","),
	})
	emitEvent("prep_completed", "Preparation context ready", map[string]string{
		"tools":              strings.Join(plan.Tools, ","),
		"memory_records":     fmt.Sprintf("%d", len(memoryCtx.Records)),
		"handoff_briefs":     fmt.Sprintf("%d", len(prep.SessionMemories)),
		"evidence_items":     fmt.Sprintf("%d", len(prep.Bundle.Evidence)),
		"context_policy":     "minimum_necessary_advisory_context",
		"continuation_ready": "true",
	})
	return prep
}

func (a *App) prepareCodebaseRouteBrief(ctx context.Context, activeDirectory, input string, emitEvent func(string, string, map[string]string)) (SessionMemory, TaskRoute, bool) {
	workspace := strings.TrimSpace(activeDirectory)
	if workspace == "" {
		return SessionMemory{}, TaskRoute{}, false
	}
	emitEvent("prep_workspace_scan_started", "Inspecting workspace for codebase route", map[string]string{
		"workspace": workspace,
	})
	cm, err := UpdateCodebaseMap(workspace, DefaultCodebaseMapPath(workspace), CodebaseMapConfig{MaxFiles: 1200})
	if err != nil {
		emitEvent("prep_workspace_scan_failed", "Workspace route preparation failed", map[string]string{
			"workspace": workspace,
			"error":     truncateOutput(err.Error()),
		})
		return SessionMemory{}, TaskRoute{}, false
	}
	if err := WriteCodebaseMap(cm, DefaultCodebaseMapPath(workspace)); err != nil {
		emitEvent("prep_workspace_scan_failed", "Codebase map write failed", map[string]string{
			"workspace": workspace,
			"error":     truncateOutput(err.Error()),
		})
		return SessionMemory{}, TaskRoute{}, false
	}
	route := RouteTaskWithCodebaseMap(cm, input)
	routeContent := formatCodebaseRouteBrief(route)
	if a != nil && a.memory != nil && strings.TrimSpace(routeContent) != "" {
		if err := a.memory.EnsureSchema(ctx); err == nil {
			if _, err := a.memory.AddMemory(ctx, "codebase_context_manager", "codebase_route_brief", routeContent, []string{"prep-context", "codebase-route", "file-chunks", "workspace:" + workspaceHash(workspace)}); err == nil {
				emitEvent("prep_codebase_route_memory_stored", "Codebase route and chunk context stored in memory", map[string]string{
					"workspace": workspace,
					"chunks":    fmt.Sprintf("%d", len(route.FileChunks)),
				})
			}
		}
	}
	emitEvent("prep_workspace_scan_completed", "Codebase route prepared from workspace evidence", map[string]string{
		"workspace":      workspace,
		"files":          fmt.Sprintf("%d", len(cm.Files)),
		"modules":        fmt.Sprintf("%d", len(cm.Modules)),
		"chunks":         fmt.Sprintf("%d", len(route.FileChunks)),
		"likely_files":   strings.Join(route.LikelyFiles, ","),
		"verification":   strings.Join(route.VerificationCommands, ","),
		"confidence":     fmt.Sprintf("%d", route.Confidence),
		"evidence_file":  DefaultCodebaseMapPath(workspace),
		"continuable_by": "codebase-map",
	})
	return SessionMemory{
		Kind:    "codebase_route_brief",
		Content: routeContent,
		Tags:    []string{"prep-context", "codebase-route", "workspace:" + workspaceHash(workspace)},
	}, route, true
}

func (a *App) prepareDocumentationBrief(ctx context.Context, input string, tags []string, plan ContextToolPlan, emitEvent func(string, string, map[string]string)) (SessionMemory, bool) {
	if !plan.NeedsDocuments || a == nil {
		return SessionMemory{}, false
	}
	searchTags := cleanMemoryTags(append([]string{"documentation"}, tags...))
	if a.memory != nil {
		emitEvent("documentation_memory_search_started", "Documentation specialist checking documentation memory", map[string]string{
			"query": input,
			"tags":  strings.Join(searchTags, ","),
			"role":  "documentation_specialist",
		})
		answer, err := AnswerDocumentationQuestionFromMemory(ctx, input, a.memory, searchTags, 4)
		if err != nil {
			emitEvent("documentation_memory_search_failed", "Documentation memory search failed", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else if !answer.NeedsScrape {
			emitEvent("documentation_brief_loaded", "Documentation specialist loaded reusable guidance", map[string]string{
				"sources": fmt.Sprintf("%d", len(answer.Brief.Sources)),
				"role":    "documentation_specialist",
			})
			return SessionMemory{
				Kind:      "documentation_brief",
				Content:   answer.Answer,
				Tags:      cleanMemoryTags(append([]string{"prep-context", "documentation"}, tags...)),
				CreatedAt: nowUTC(),
			}, true
		} else {
			emitEvent("documentation_research_needed", "Documentation specialist found no reusable brief", map[string]string{
				"query":  input,
				"reason": "no_matching_documentation_memory",
			})
		}
	} else {
		emitEvent("documentation_research_needed", "Documentation specialist has no memory store; fetching authoritative docs", map[string]string{
			"query":  input,
			"reason": "memory_unavailable",
		})
	}

	target := InferDocumentationResearchTarget(input)
	if len(target.Sources) == 0 {
		emitEvent("documentation_research_skipped", "Documentation specialist has no authoritative source route", map[string]string{
			"query": input,
		})
		return SessionMemory{}, false
	}

	emitEvent("documentation_web_research_started", "Documentation specialist fetching authoritative docs", map[string]string{
		"query":   input,
		"sources": strings.Join(webDocSourceURLs(target.Sources), "\n"),
		"role":    "documentation_specialist",
	})
	research, err := ResearchWebDocs(ctx, input, target.Sources, target.Queries, WebDocResearchConfig{
		FetchTimeout: 20 * time.Second,
		ChunkConfig:  DocumentSearchConfig{ChunkChars: 2400, ChunkOverlap: 300},
		MaxHits:      8,
	})
	if err != nil {
		emitEvent("documentation_web_research_failed", "Documentation specialist web research failed", map[string]string{
			"error": truncateOutput(err.Error()),
		})
		return SessionMemory{}, false
	}
	if len(research.Hits) == 0 {
		emitEvent("documentation_web_research_empty", "Documentation specialist found no matching excerpts", map[string]string{
			"sources": fmt.Sprintf("%d", len(research.Sources)),
			"queries": strings.Join(target.Queries, " | "),
		})
		return SessionMemory{}, false
	}

	if a.memory != nil {
		if err := storeDocResearchHits(ctx, a.memory, input, research.Hits, append(searchTags, target.Tags...)); err != nil {
			emitEvent("documentation_memory_store_failed", "Documentation specialist could not store fetched docs", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else {
			emitEvent("documentation_memory_stored", "Documentation specialist stored fetched docs", map[string]string{
				"hits": fmt.Sprintf("%d", len(research.Hits)),
			})
		}
	}

	content := FormatDocumentationAuthorityBrief(BuildDocumentationAuthorityBrief(input, docResearchHitsAsMemories(input, research.Hits)))
	emitEvent("documentation_brief_loaded", "Documentation specialist loaded fetched guidance", map[string]string{
		"sources": fmt.Sprintf("%d", len(research.Sources)),
		"hits":    fmt.Sprintf("%d", len(research.Hits)),
		"role":    "documentation_specialist",
	})
	return SessionMemory{
		Kind:      "documentation_brief",
		Content:   content,
		Tags:      cleanMemoryTags(append([]string{"prep-context", "documentation"}, append(tags, target.Tags...)...)),
		CreatedAt: nowUTC(),
	}, true
}

func (a *App) prepareWebResearchBrief(ctx context.Context, input string, plan ContextToolPlan, emitEvent func(string, string, map[string]string)) (SessionMemory, bool) {
	if !plan.NeedsWebResearch {
		return SessionMemory{}, false
	}
	events, observation := a.autoResearchForTurn(ctx, input, plan)
	for _, event := range events {
		emitEvent(event.Type, event.Summary, event.Details)
	}
	if observation == nil || strings.TrimSpace(observation.Stdout) == "" {
		return SessionMemory{}, false
	}
	return SessionMemory{
		Kind: "web_research_brief",
		Content: strings.TrimSpace(strings.Join([]string{
			"WEB_RESEARCH_BRIEF",
			"query: " + strings.TrimSpace(input),
			"content:",
			observation.Stdout,
		}, "\n")),
		Tags:      cleanMemoryTags(append([]string{"prep-context", "web-research"}, researchTagsFromQuery(input)...)),
		CreatedAt: nowUTC(),
	}, true
}

func formatCodebaseRouteBrief(route TaskRoute) string {
	lines := []string{
		"CODEBASE_ROUTE_BRIEF",
		"intent: " + strings.TrimSpace(route.Intent),
		fmt.Sprintf("confidence: %d", route.Confidence),
	}
	if len(route.LikelyFiles) > 0 {
		lines = append(lines, "likely_files: "+strings.Join(route.LikelyFiles, ", "))
	}
	if len(route.RelevantModules) > 0 {
		lines = append(lines, "relevant_modules: "+strings.Join(route.RelevantModules, ", "))
	}
	if len(route.VerificationCommands) > 0 {
		lines = append(lines, "verification_commands: "+strings.Join(route.VerificationCommands, " | "))
	}
	if len(route.KnownRisks) > 0 {
		lines = append(lines, "known_risks: "+strings.Join(route.KnownRisks, " | "))
	}
	if len(route.Reasons) > 0 {
		lines = append(lines, "reasons: "+strings.Join(route.Reasons, " | "))
	}
	if route.ContextPolicy != "" {
		lines = append(lines, "context_policy: "+route.ContextPolicy)
	}
	if len(route.FileChunks) > 0 {
		lines = append(lines, "file_chunks:")
		for _, chunk := range route.FileChunks {
			lines = append(lines,
				fmt.Sprintf("- id: %s", chunk.ID),
				fmt.Sprintf("  path: %s", chunk.Path),
				fmt.Sprintf("  lines: %d-%d", chunk.StartLine, chunk.EndLine),
				fmt.Sprintf("  sed: %s", chunk.SedCommand),
				fmt.Sprintf("  reason: %s", chunk.Reason),
			)
			if strings.TrimSpace(chunk.Preview) != "" {
				lines = append(lines, "  preview: |")
				for _, line := range strings.Split(chunk.Preview, "\n") {
					lines = append(lines, "    "+line)
				}
			}
		}
		lines = append(lines,
			"chunk_editing_rules:",
			"- never request or load the full file when file_chunks are available",
			"- edit one chunk or adjacent chunk range at a time using the line anchors",
			"- use the provided sed command for readback before constructing sed/perl/patch edits",
			"- after each chunk edit, verify the changed line range and continue to the next needed chunk",
		)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (a *App) autoResearchForTurn(ctx context.Context, input string, plan ContextToolPlan) ([]Event, *CommandObservation) {
	if !plan.NeedsWebResearch {
		return nil, nil
	}
	queries := BuildWebResearchQueries(input, plan, 4)
	if len(queries) == 0 {
		return nil, nil
	}
	query := queries[0]
	events := []Event{}
	events = append(events, a.newEvent("auto_research_started", "Automatic web research started", map[string]string{
		"query":   query,
		"queries": strings.Join(queries, " | "),
	}))
	memoryContext := ""
	if a.memory != nil {
		memoryRecords, err := searchAutoResearchMemory(ctx, a.memory, queries, 4)
		if err != nil {
			events = append(events, a.newEvent("auto_research_memory_lookup_failed", "Automatic research memory lookup failed", map[string]string{"error": truncateOutput(err.Error())}))
		} else if len(memoryRecords) > 0 {
			memoryContext = formatAutoResearchMemoryContext(memoryRecords, 2400)
			events = append(events, a.newEvent("auto_research_memory_loaded", "Automatic research loaded relevant memory before web search", map[string]string{
				"records": fmt.Sprintf("%d", len(memoryRecords)),
				"kinds":   strings.Join(memoryRecordKinds(memoryRecords), ","),
				"ids":     strings.Join(memoryRecordIDs(memoryRecords), ","),
			}))
		}
	}
	if a.web == nil {
		events = append(events, a.newEvent("auto_research_skipped", "Web search service is unavailable", nil))
		if strings.TrimSpace(memoryContext) == "" {
			return events, nil
		}
		return events, &CommandObservation{
			Step:    0,
			Command: "AUTO_RESEARCH_MEMORY: " + query,
			Status:  "success",
			Stdout:  truncateForObservation(memoryContext, defaultAgentObservationChars),
		}
	}
	searchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results := []websearch.Result{}
	searchErrors := []string{}
	for _, candidate := range queries {
		events = append(events, a.newEvent("web_search_started", "Web search started", map[string]string{
			"query": candidate,
			"role":  "web_research_specialist",
		}))
		candidateResults, err := a.web.SearchAll(searchCtx, candidate)
		if err != nil {
			searchErrors = append(searchErrors, candidate+": "+err.Error())
			continue
		}
		events = append(events, a.newEvent("web_search_completed", "Web search completed", webSearchTimelineDetails(candidate, candidateResults)))
		results = append(results, candidateResults...)
		if len(results) >= 8 {
			break
		}
	}
	results = dedupeWebSearchResults(results)
	if len(results) == 0 {
		detail := "no search results"
		if len(searchErrors) > 0 {
			detail = strings.Join(searchErrors, " | ")
		}
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research failed", map[string]string{"error": detail}))
		return events, nil
	}
	contextText := strings.TrimSpace(strings.Join([]string{memoryContext, websearch.BuildContext(results, 5000)}, "\n\n"))
	if strings.TrimSpace(contextText) == "" {
		events = append(events, a.newEvent("auto_research_failed", "Automatic web research returned empty context", nil))
		return events, nil
	}
	events = append(events, a.newEvent("auto_research_completed", "Automatic web research context captured", map[string]string{
		"query":       query,
		"queries":     strings.Join(queries, " | "),
		"results":     fmt.Sprintf("%d", len(results)),
		"result_urls": strings.Join(webSearchResultURLs(results, 5), "\n"),
	}))

	if a.memory != nil {
		if result, storeErr := ResearchWebToMemory(ctx, query, staticWebSearchResults{results: results}, a.memory, WebResearchMemoryConfig{
			AgentID: "omni_auto_researcher",
			Tags:    researchTagsFromQuery(query),
		}); storeErr != nil {
			events = append(events, a.newEvent("auto_research_memory_failed", "Automatic research memory write failed", map[string]string{"error": storeErr.Error()}))
		} else {
			events = append(events, a.newEvent("auto_research_memory_stored", "Automatic research stored in Postgres memory", map[string]string{
				"stored": fmt.Sprintf("%d", result.StoredCount),
			}))
		}
	}

	return events, &CommandObservation{
		Step:    0,
		Command: "AUTO_RESEARCH: " + query,
		Status:  "success",
		Stdout:  truncateForObservation(contextText, defaultAgentObservationChars),
	}
}

func searchAutoResearchMemory(ctx context.Context, memory *PGMemoryStore, queries []string, limit int) ([]MemoryRecord, error) {
	if memory == nil {
		return nil, nil
	}
	if err := memory.EnsureSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 4
	}
	seen := map[int64]bool{}
	out := []MemoryRecord{}
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		records, err := memory.SearchMemory(ctx, query, []string{"web", "research", "documentation"}, limit)
		if err != nil {
			continue
		}
		for _, record := range records {
			if record.ID > 0 && seen[record.ID] {
				continue
			}
			if record.ID > 0 {
				seen[record.ID] = true
			}
			out = append(out, record)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func (a *App) buildThinkingToolDeps() ThinkingToolDeps {
	deps := ThinkingToolDeps{WebSearch: a.web}
	if a.memory == nil {
		return deps
	}
	memory := a.memory
	deps.MemorySearch = func(searchCtx context.Context, query string) (string, error) {
		query = strings.TrimSpace(query)
		if query == "" {
			return "memory_search requires a query", nil
		}
		if err := memory.EnsureSchema(searchCtx); err != nil {
			return "", err
		}
		queries := BuildWebResearchQueries(query, ContextToolPlan{NeedsWebResearch: true}, 3)
		if len(queries) == 0 {
			queries = []string{query}
		}
		records, err := searchAutoResearchMemory(searchCtx, memory, queries, 6)
		if err != nil {
			return "", err
		}
		if len(records) == 0 {
			records, err = memory.SearchMemory(searchCtx, query, nil, 6)
			if err != nil {
				return "", err
			}
		}
		text := formatAutoResearchMemoryContext(records, 4000)
		if strings.TrimSpace(text) == "" {
			return "no memory matches for: " + query, nil
		}
		return text, nil
	}
	return deps
}

func formatAutoResearchMemoryContext(records []MemoryRecord, budget int) string {
	if len(records) == 0 {
		return ""
	}
	lines := []string{"MEMORY_RESEARCH_CONTEXT"}
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if content == "" {
			continue
		}
		segment := strings.Join([]string{
			fmt.Sprintf("memory_id: %d", record.ID),
			"kind: " + firstNonEmpty(record.Kind, "memory"),
			"tags: " + strings.Join(cleanMemoryTags(record.Tags), ","),
			"content:",
			truncateForStructuredContext(content, 900),
		}, "\n")
		if budget > 0 && len(strings.Join(append(lines, segment), "\n\n")) > budget {
			break
		}
		lines = append(lines, segment)
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n"))
}
