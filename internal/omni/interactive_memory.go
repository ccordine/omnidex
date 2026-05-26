package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const interactiveMemoryLimit = 6

type PromptTagger interface {
	TagPrompt(ctx context.Context, input PromptTagInput) (PromptTagResult, error)
}

type PromptTagInput struct {
	Prompt                  string
	CurrentWorkingDirectory string
	MaxTags                 int
}

type PromptTagResult struct {
	Tags []string
}

type OllamaPromptTagger struct {
	client CommandDecisionClient
}

func NewOllamaPromptTagger(client CommandDecisionClient) *OllamaPromptTagger {
	return &OllamaPromptTagger{client: client}
}

func (t *OllamaPromptTagger) TagPrompt(ctx context.Context, input PromptTagInput) (PromptTagResult, error) {
	if t == nil || t.client == nil {
		return PromptTagResult{}, fmt.Errorf("prompt tagger requires an LLM client")
	}
	maxTags := input.MaxTags
	if maxTags <= 0 {
		maxTags = 8
	}
	req := OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are Omni's intent tagging specialist.",
					"Extract compact relevance tags for memory retrieval only.",
					"Return only comma-separated lowercase tags.",
					"Do not add requirements, dependencies, frameworks, tools, services, files, or commands.",
					"Do not infer a user's usual stack unless the prompt explicitly asks to use preferences or history.",
				}, "\n"),
			},
			{
				Role: "user",
				Content: strings.Join([]string{
					fmt.Sprintf("Maximum tags: %d", maxTags),
					"Current working directory: " + strings.TrimSpace(input.CurrentWorkingDirectory),
					"Prompt:",
					strings.TrimSpace(input.Prompt),
				}, "\n"),
			},
		},
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}
	resp, err := t.client.ChatRaw(ctx, req)
	if err != nil {
		return PromptTagResult{}, err
	}
	return PromptTagResult{Tags: cleanMemoryTags(strings.Split(resp.Content, ","))}, nil
}

type interactiveMemoryContext struct {
	Tags     []string
	Records  []MemoryRecord
	Memories []SessionMemory
}

func (a *App) loadInteractiveMemoryContext(ctx context.Context, prompt, activeDirectory string, emitEvent func(string, string, map[string]string)) interactiveMemoryContext {
	if a == nil || a.memory == nil {
		return interactiveMemoryContext{}
	}
	tagger := a.promptTagger
	if tagger == nil && a.planner != nil {
		tagger = NewOllamaPromptTagger(a.planner)
	}
	if tagger == nil {
		emitEvent("memory_context_skipped", "Interactive memory retrieval skipped", map[string]string{
			"reason": "prompt_tagger_unavailable",
		})
		return interactiveMemoryContext{}
	}
	if err := a.memory.EnsureSchema(ctx); err != nil {
		emitEvent("memory_context_skipped", "Interactive memory retrieval skipped", map[string]string{
			"reason": "schema_error",
			"error":  truncateOutput(err.Error()),
		})
		return interactiveMemoryContext{}
	}
	emitEvent("memory_tagging_started", "Prompt tagging specialist started", map[string]string{
		"active_directory": activeDirectory,
		"max_tags":         "8",
	})
	tagResult, err := tagger.TagPrompt(ctx, PromptTagInput{
		Prompt:                  prompt,
		CurrentWorkingDirectory: activeDirectory,
		MaxTags:                 8,
	})
	if err != nil {
		emitEvent("memory_tagging_failed", "Prompt tagging specialist failed", map[string]string{
			"error": truncateOutput(err.Error()),
		})
		return interactiveMemoryContext{}
	}
	tags := cleanMemoryTags(tagResult.Tags)
	if len(tags) == 0 {
		emitEvent("memory_context_skipped", "Interactive memory retrieval skipped", map[string]string{
			"reason": "no_tags",
		})
		return interactiveMemoryContext{}
	}
	emitEvent("memory_tags_generated", "Prompt tags generated for memory retrieval", map[string]string{
		"tags": strings.Join(tags, ","),
	})

	query := executionMemorySearchQuery(prompt, tags)
	workspaceScopedTag := ""
	if strings.TrimSpace(activeDirectory) != "" {
		workspaceScopedTag = "workspace:" + workspaceHash(activeDirectory)
	}
	searchTags := executionMemorySearchTags(tags, workspaceScopedTag)
	emitEvent("memory_search_started", "Searching Postgres memory for relevant context", map[string]string{
		"query": query,
		"tags":  strings.Join(searchTags, ","),
		"limit": fmt.Sprintf("%d", interactiveMemoryLimit),
		"role":  "memory_retrieval_specialist",
	})
	records, err := a.memory.SearchMemory(ctx, query, searchTags, interactiveMemoryLimit*4)
	if err != nil {
		emitEvent("memory_context_skipped", "Interactive memory retrieval skipped", map[string]string{
			"reason": "search_error",
			"error":  truncateOutput(err.Error()),
		})
		return interactiveMemoryContext{Tags: tags}
	}
	records, excluded := filterExecutionMemoryRecords(records, prompt, activeDirectory, interactiveMemoryLimit)
	if excluded > 0 {
		emitEvent("memory_context_filtered", "Execution memory context filtered", map[string]string{
			"excluded":       fmt.Sprintf("%d", excluded),
			"workspace_hash": workspaceHash(activeDirectory),
			"reason":         "execution_authority_or_foreign_project",
		})
	}
	memories := memoryRecordsToSessionMemoriesWithScope(records, prompt, activeDirectory)
	emitEvent("memory_context_loaded", "Interactive memory context loaded", map[string]string{
		"matches": fmt.Sprintf("%d", len(records)),
		"ids":     strings.Join(memoryRecordIDs(records), ","),
		"kinds":   strings.Join(memoryRecordKinds(records), ","),
	})
	return interactiveMemoryContext{Tags: tags, Records: records, Memories: memories}
}

func executionMemorySearchTags(promptTags []string, workspaceScopedTag string) []string {
	tags := append([]string{}, cleanMemoryTags(promptTags)...)
	tags = append(tags,
		"capability",
		"capability-memory",
		"expertise-memory",
		"documentation",
		"documentation-brief",
		"doc-research",
		"research-index-memory",
		"source-memory",
		"validated-playbook",
		"procedure-memory",
	)
	if strings.TrimSpace(workspaceScopedTag) != "" {
		tags = append(tags, workspaceScopedTag)
	}
	return cleanMemoryTags(tags)
}

func executionMemorySearchQuery(prompt string, tags []string) string {
	lowerPrompt := strings.ToLower(prompt)
	preferred := []string{"react", "vite", "node", "npm", "docker", "ollama", "go", "rust", "cargo", "zig", "python", "research", "memory"}
	for _, candidate := range preferred {
		if strings.Contains(lowerPrompt, candidate) {
			return candidate
		}
	}
	for _, tag := range cleanMemoryTags(tags) {
		if len(tag) >= 3 && !strings.HasPrefix(tag, "workspace:") {
			return tag
		}
	}
	for _, token := range strings.FieldsFunc(lowerPrompt, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(token) >= 4 {
			return token
		}
	}
	return "task"
}

func filterExecutionMemoryRecords(records []MemoryRecord, prompt, activeDirectory string, limit int) ([]MemoryRecord, int) {
	if limit <= 0 {
		limit = interactiveMemoryLimit
	}
	workspaceScopedTag := ""
	if strings.TrimSpace(activeDirectory) != "" {
		workspaceScopedTag = "workspace:" + workspaceHash(activeDirectory)
	}
	identities := currentProjectIdentityTokens(activeDirectory, prompt)
	out := make([]MemoryRecord, 0, minInt(limit, len(records)))
	excluded := 0
	for _, record := range records {
		if !executionMemoryRecordAllowed(record, workspaceScopedTag) || memoryRecordLooksForeignProject(record, identities, workspaceScopedTag) {
			excluded++
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			excluded += len(records) - len(out) - excluded
			break
		}
	}
	return out, excluded
}

func executionMemoryRecordAllowed(record MemoryRecord, workspaceScopedTag string) bool {
	kind := strings.ToLower(strings.TrimSpace(record.Kind))
	tags := cleanMemoryTags(record.Tags)
	tagSet := map[string]bool{}
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if tagSet["prompt"] || tagSet["response"] || kind == "episodic" {
		return false
	}
	switch kind {
	case MemoryKindCapability, "capability", MemoryKindExpertise, MemoryKindSource, MemoryKindResearchIndex,
		"documentation_brief", "documentation_research", "web_research_brief", "expertise_research":
		return true
	case validatedPlaybookKind, MemoryKindProcedural:
		if kind == validatedPlaybookKind && !validatedPlaybookRecordIsNormalizedAdvisory(record) {
			return false
		}
		return tagSet["advisory-only"] || tagSet["validated-playbook"] || tagSet["procedure-memory"]
	case MemoryKindProject, "codebase_route", "codebase_route_brief", "worksite_survey":
		return workspaceScopedTag != "" && tagSet[workspaceScopedTag]
	default:
		return tagSet["capability-memory"] ||
			tagSet["expertise-memory"] ||
			tagSet["documentation-brief"] ||
			tagSet["doc-research"] ||
			tagSet["research-index-memory"] ||
			tagSet["source-memory"]
	}
}

func memoryRecordLooksForeignProject(record MemoryRecord, identities map[string]bool, workspaceScopedTag string) bool {
	tags := cleanMemoryTags(record.Tags)
	for _, tag := range tags {
		if strings.HasPrefix(tag, "workspace:") && workspaceScopedTag != "" && tag != workspaceScopedTag {
			return true
		}
	}
	text := strings.ToLower(record.Content + " " + strings.Join(tags, " "))
	if strings.TrimSpace(record.Kind) == validatedPlaybookKind && validatedPlaybookRecordIsNormalizedAdvisory(record) {
		return false
	}
	for _, marker := range []string{"fruityloops", "fruitmixer"} {
		if strings.Contains(text, marker) && !identities[marker] {
			return true
		}
	}
	if strings.Contains(text, "foreign_project_memory") || strings.Contains(text, "foreign-project-memory") {
		return true
	}
	return false
}

func validatedPlaybookRecordIsNormalizedAdvisory(record MemoryRecord) bool {
	var playbook ValidatedPlaybook
	if json.Unmarshal([]byte(strings.TrimSpace(record.Content)), &playbook) != nil {
		return false
	}
	if !strings.Contains(strings.ToLower(playbook.ScopePolicy), "advisory") {
		return false
	}
	task := strings.ToLower(playbook.TaskPattern)
	return !strings.Contains(task, "fruityloops") && !strings.Contains(task, "fruitmixer")
}

func currentProjectIdentityTokens(activeDirectory, prompt string) map[string]bool {
	out := map[string]bool{}
	for _, source := range []string{prompt, filepath.Base(strings.TrimSpace(activeDirectory))} {
		for _, token := range strings.FieldsFunc(strings.ToLower(source), func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
		}) {
			if len(token) >= 3 {
				out[token] = true
			}
		}
	}
	if strings.TrimSpace(activeDirectory) != "" {
		for token := range packageNameTokens(activeDirectory) {
			out[token] = true
		}
		if gitTop, err := gitRepoTopLevel(activeDirectory); err == nil {
			for _, token := range projectIdentityTokensFromText(filepath.Base(gitTop)) {
				out[token] = true
			}
		}
		blob, err := os.ReadFile(filepath.Join(activeDirectory, "package.json"))
		if err == nil {
			var pkg struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(blob, &pkg) == nil {
				for _, token := range strings.FieldsFunc(strings.ToLower(pkg.Name), func(r rune) bool {
					return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
				}) {
					if len(token) >= 3 {
						out[token] = true
					}
				}
			}
		}
	}
	return out
}

func memoryRecordIDs(records []MemoryRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		if record.ID > 0 {
			out = append(out, fmt.Sprintf("%d", record.ID))
		}
	}
	return out
}

func memoryRecordKinds(records []MemoryRecord) []string {
	out := make([]string, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		kind := strings.TrimSpace(record.Kind)
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}

func memoryRecordsToSessionMemories(records []MemoryRecord) []SessionMemory {
	return memoryRecordsToSessionMemoriesWithScope(records, "", "")
}

func memoryRecordsToSessionMemoriesWithScope(records []MemoryRecord, prompt, activeDirectory string) []SessionMemory {
	if len(records) == 0 {
		return nil
	}
	workspaceScopedTag := ""
	if strings.TrimSpace(activeDirectory) != "" {
		workspaceScopedTag = "workspace:" + workspaceHash(activeDirectory)
	}
	identities := currentProjectIdentityTokens(activeDirectory, prompt)
	out := make([]SessionMemory, 0, len(records))
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if content == "" {
			continue
		}
		createdAt := ""
		if !record.CreatedAt.IsZero() {
			createdAt = record.CreatedAt.Format(time.RFC3339)
		}
		out = append(out, SessionMemory{
			Kind:      firstNonEmpty(record.Kind, "episodic"),
			Content:   content,
			Tags:      memoryAuthorityTags(SessionMemory{Kind: record.Kind, Content: content, Tags: record.Tags}, identities, workspaceScopedTag),
			CreatedAt: createdAt,
		})
	}
	return out
}

func (a *App) persistInteractiveTurnMemory(ctx context.Context, turnID, prompt, response string, tags []string, result CommandDecisionResult, emitEvent func(string, string, map[string]string)) {
	if a == nil || a.memory == nil {
		return
	}
	tags = cleanMemoryTags(tags)
	if len(tags) == 0 {
		return
	}
	if err := a.memory.EnsureSchema(ctx); err != nil {
		emitEvent("memory_turn_persist_failed", "Interactive turn memory persistence failed", map[string]string{
			"error": truncateOutput(err.Error()),
		})
		return
	}
	persistTags := append([]string{"interactive-turn"}, tags...)
	count := 0
	if strings.TrimSpace(prompt) != "" {
		if _, err := a.memory.AddMemory(ctx, "prompt_tagger", "episodic", prompt, append([]string{"prompt"}, persistTags...)); err != nil {
			emitEvent("memory_turn_persist_failed", "Interactive prompt memory persistence failed", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else {
			count++
		}
	}
	if strings.TrimSpace(response) != "" {
		if _, err := a.memory.AddMemory(ctx, "response_specialist", "episodic", response, append([]string{"response"}, persistTags...)); err != nil {
			emitEvent("memory_turn_persist_failed", "Interactive response memory persistence failed", map[string]string{
				"error": truncateOutput(err.Error()),
			})
		} else {
			count++
		}
	}
	capabilityCount := 0
	playbookCount := 0
	for _, obs := range result.Observations {
		content := strings.TrimSpace(obs.CapabilityMemory)
		if content == "" {
			continue
		}
		if _, err := a.memory.AddMemory(ctx, "memory_specialist", "capability", content, append([]string{"capability", "evidence-backed"}, persistTags...)); err == nil {
			capabilityCount++
		}
	}
	if playbook, ok := extractValidatedPlaybook(prompt, result, "structured_planner"); ok {
		if _, err := a.memory.AddMemory(ctx, "procedure_memory_specialist", validatedPlaybookKind, playbook.Content, append([]string{"validated-playbook", "procedure-memory", "advisory-only"}, append(persistTags, playbook.Tags...)...)); err == nil {
			playbookCount++
		}
	}
	emitEvent("memory_turn_persisted", "Interactive turn memory persisted", map[string]string{
		"turn_id":             turnID,
		"records":             fmt.Sprintf("%d", count),
		"capability_records":  fmt.Sprintf("%d", capabilityCount),
		"playbook_records":    fmt.Sprintf("%d", playbookCount),
		"tags":                strings.Join(tags, ","),
		"memory_scope_policy": "advisory_context_only",
	})
}
