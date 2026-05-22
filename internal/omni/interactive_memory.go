package omni

import (
	"context"
	"fmt"
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

	emitEvent("memory_search_started", "Searching Postgres memory for relevant context", map[string]string{
		"query": "",
		"tags":  strings.Join(tags, ","),
		"limit": fmt.Sprintf("%d", interactiveMemoryLimit),
		"role":  "memory_retrieval_specialist",
	})
	searchTags := append([]string{}, tags...)
	searchTags = append(searchTags, "validated-playbook", "procedure-memory")
	records, err := a.memory.SearchMemory(ctx, "", searchTags, interactiveMemoryLimit)
	if err != nil {
		emitEvent("memory_context_skipped", "Interactive memory retrieval skipped", map[string]string{
			"reason": "search_error",
			"error":  truncateOutput(err.Error()),
		})
		return interactiveMemoryContext{Tags: tags}
	}
	memories := memoryRecordsToSessionMemories(records)
	emitEvent("memory_context_loaded", "Interactive memory context loaded", map[string]string{
		"matches": fmt.Sprintf("%d", len(records)),
		"ids":     strings.Join(memoryRecordIDs(records), ","),
		"kinds":   strings.Join(memoryRecordKinds(records), ","),
	})
	return interactiveMemoryContext{Tags: tags, Records: records, Memories: memories}
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
	if len(records) == 0 {
		return nil
	}
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
			Tags:      cleanMemoryTags(record.Tags),
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
