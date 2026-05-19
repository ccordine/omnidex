package odn

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

const (
	defaultExpertiseAgentID = "odn_expertise_manager"
	defaultExpertiseKind    = "expertise_research"
)

type ExpertiseResearchPlan struct {
	Subject         string   `json:"subject"`
	ResearchQueries []string `json:"research_queries"`
	AdjacentTopics  []string `json:"adjacent_topics"`
	SuccessCriteria []string `json:"success_criteria"`
}

type ExpertiseResearchConfig struct {
	AgentID        string
	Kind           string
	MaxQueries     int
	MaxResultsEach int
	ChunkSize      int
}

type ExpertiseResearchResult struct {
	Plan           ExpertiseResearchPlan
	QueryResults   map[string][]websearch.Result
	StoredMemories []MemoryRecord
	StoredCount    int
}

type ExpertiseRecallResult struct {
	Subject       string
	Question      string
	Direct        []MemoryRecord
	Related       []MemoryRecord
	UsedMemory    bool
	NeedsResearch bool
	Answer        string
}

func BuildExpertiseResearchOmnibus(ctx context.Context, subject string, planner DBManagerLLMClient, searcher WebSearchService, memory *PGMemoryStore, cfg ExpertiseResearchConfig) (ExpertiseResearchResult, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ExpertiseResearchResult{}, fmt.Errorf("expertise subject is required")
	}
	if planner == nil {
		return ExpertiseResearchResult{}, fmt.Errorf("expertise planner llm is required")
	}
	if searcher == nil {
		return ExpertiseResearchResult{}, fmt.Errorf("web search service is required")
	}
	if memory == nil {
		return ExpertiseResearchResult{}, fmt.Errorf("memory store is required")
	}
	cfg = normalizeExpertiseResearchConfig(cfg)

	plan, err := PlanExpertiseResearch(ctx, subject, planner, cfg)
	if err != nil {
		return ExpertiseResearchResult{}, err
	}
	if err := memory.EnsureSchema(ctx); err != nil {
		return ExpertiseResearchResult{}, err
	}

	out := ExpertiseResearchResult{Plan: plan, QueryResults: map[string][]websearch.Result{}}
	for _, query := range plan.ResearchQueries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		results, err := searcher.SearchAll(ctx, query)
		if err != nil {
			return out, err
		}
		if cfg.MaxResultsEach > 0 && len(results) > cfg.MaxResultsEach {
			results = results[:cfg.MaxResultsEach]
		}
		out.QueryResults[query] = results
		for _, result := range results {
			content := formatExpertiseSourceMemory(plan, query, result, cfg.ChunkSize)
			if strings.TrimSpace(content) == "" {
				continue
			}
			record, err := memory.AddMemory(ctx, cfg.AgentID, cfg.Kind, content, expertiseMemoryTags(plan, query, result))
			if err != nil {
				return out, err
			}
			out.StoredMemories = append(out.StoredMemories, record)
			out.StoredCount++
		}
	}
	omnibus := formatExpertiseOmnibusMemory(plan, out.QueryResults)
	record, err := memory.AddMemory(ctx, cfg.AgentID, cfg.Kind, omnibus, expertiseOmnibusTags(plan))
	if err != nil {
		return out, err
	}
	out.StoredMemories = append(out.StoredMemories, record)
	out.StoredCount++
	return out, nil
}

func PlanExpertiseResearch(ctx context.Context, subject string, planner DBManagerLLMClient, cfg ExpertiseResearchConfig) (ExpertiseResearchPlan, error) {
	resp, err := planner.ChatRaw(ctx, buildExpertisePlannerRequest(subject, cfg))
	if err != nil {
		return ExpertiseResearchPlan{}, err
	}
	var plan ExpertiseResearchPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &plan); err != nil {
		return plan, fmt.Errorf("parse expertise research plan: %w", err)
	}
	plan.Subject = strings.TrimSpace(plan.Subject)
	if plan.Subject == "" {
		plan.Subject = strings.TrimSpace(subject)
	}
	plan.ResearchQueries = dedupeStrings(plan.ResearchQueries)
	plan.AdjacentTopics = dedupeStrings(plan.AdjacentTopics)
	plan.SuccessCriteria = dedupeStrings(plan.SuccessCriteria)
	if len(plan.ResearchQueries) == 0 {
		return plan, fmt.Errorf("expertise research plan contains no research queries")
	}
	if cfg.MaxQueries > 0 && len(plan.ResearchQueries) > cfg.MaxQueries {
		plan.ResearchQueries = plan.ResearchQueries[:cfg.MaxQueries]
	}
	return plan, nil
}

func RecallExpertiseFromMemory(ctx context.Context, subject, question string, memory *PGMemoryStore, limit int) (ExpertiseRecallResult, error) {
	subject = strings.TrimSpace(subject)
	question = strings.TrimSpace(question)
	if subject == "" {
		return ExpertiseRecallResult{}, fmt.Errorf("expertise subject is required")
	}
	if question == "" {
		return ExpertiseRecallResult{}, fmt.Errorf("question is required")
	}
	if memory == nil {
		return ExpertiseRecallResult{}, fmt.Errorf("memory store is required")
	}
	if limit <= 0 {
		limit = 8
	}
	subjectTag := "expertise:" + subject
	direct, err := memory.SearchMemory(ctx, question, []string{subjectTag}, limit)
	if err != nil {
		return ExpertiseRecallResult{}, err
	}
	if len(direct) == 0 {
		direct, err = memory.SearchMemory(ctx, "", []string{subjectTag}, limit)
		if err != nil {
			return ExpertiseRecallResult{}, err
		}
	}
	relatedTags := relatedExpertiseTagsFromRecords(direct)
	related := []MemoryRecord{}
	for _, tag := range relatedTags {
		matches, err := memory.SearchMemory(ctx, "", []string{tag}, limit)
		if err != nil {
			return ExpertiseRecallResult{}, err
		}
		related = appendUniqueMemoryRecords(related, matches...)
	}
	if len(direct) == 0 && len(related) == 0 {
		return ExpertiseRecallResult{Subject: subject, Question: question, NeedsResearch: true}, nil
	}
	return ExpertiseRecallResult{
		Subject:    subject,
		Question:   question,
		Direct:     direct,
		Related:    related,
		UsedMemory: true,
		Answer:     summarizeExpertiseRecall(subject, question, direct, related),
	}, nil
}

func buildExpertisePlannerRequest(subject string, cfg ExpertiseResearchConfig) OllamaChatRequest {
	if cfg.MaxQueries <= 0 {
		cfg.MaxQueries = 8
	}
	blob, _ := json.Marshal(map[string]any{
		"subject":     strings.TrimSpace(subject),
		"max_queries": cfg.MaxQueries,
		"goal":        "build durable sourced expertise memory with adjacent topic links",
	})
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: strings.Join([]string{
				"Return JSON only.",
				"Schema: {\"subject\":\"...\",\"research_queries\":[\"...\"],\"adjacent_topics\":[\"...\"],\"success_criteria\":[\"...\"]}.",
				"You are an expertise research manager.",
				"Plan deep web research that can become durable sourced memory.",
				"Include direct foundation queries and adjacent topic queries.",
				"Prefer authoritative documents, textbooks, documentation, course notes, and primary references.",
				"No markdown.",
			}, "\n")},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"subject":          map[string]interface{}{"type": "string"},
				"research_queries": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"adjacent_topics":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"success_criteria": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"subject", "research_queries", "adjacent_topics", "success_criteria"},
		},
		Options: map[string]interface{}{"temperature": 0},
	}
}

func normalizeExpertiseResearchConfig(cfg ExpertiseResearchConfig) ExpertiseResearchConfig {
	if strings.TrimSpace(cfg.AgentID) == "" {
		cfg.AgentID = defaultExpertiseAgentID
	}
	if strings.TrimSpace(cfg.Kind) == "" {
		cfg.Kind = defaultExpertiseKind
	}
	if cfg.MaxQueries <= 0 {
		cfg.MaxQueries = 8
	}
	if cfg.MaxResultsEach <= 0 {
		cfg.MaxResultsEach = 4
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 2200
	}
	return cfg
}

func formatExpertiseSourceMemory(plan ExpertiseResearchPlan, query string, result websearch.Result, maxContent int) string {
	body := strings.TrimSpace(result.Content)
	if body == "" {
		body = strings.TrimSpace(result.Snippet)
	}
	if body == "" {
		return ""
	}
	if maxContent > 0 && len(body) > maxContent {
		body = body[:maxContent] + "\n...[truncated]"
	}
	retrieved := result.RetrievedAt
	if retrieved.IsZero() {
		retrieved = time.Now().UTC()
	}
	return strings.TrimSpace(strings.Join([]string{
		"EXPERTISE_SOURCE_MEMORY",
		"subject: " + plan.Subject,
		"query: " + strings.TrimSpace(query),
		"title: " + strings.TrimSpace(result.Title),
		"url: " + strings.TrimSpace(result.URL),
		"provider: " + strings.TrimSpace(result.Provider),
		"retrieved_at: " + retrieved.Format(time.RFC3339),
		"content:",
		body,
	}, "\n"))
}

func formatExpertiseOmnibusMemory(plan ExpertiseResearchPlan, queryResults map[string][]websearch.Result) string {
	lines := []string{
		"EXPERTISE_OMNIBUS_MEMORY",
		"subject: " + plan.Subject,
		"adjacent_topics: " + strings.Join(plan.AdjacentTopics, ", "),
		"success_criteria: " + strings.Join(plan.SuccessCriteria, "; "),
		"sources:",
	}
	queries := make([]string, 0, len(queryResults))
	for query := range queryResults {
		queries = append(queries, query)
	}
	sort.Strings(queries)
	for _, query := range queries {
		for _, result := range queryResults[query] {
			lines = append(lines, fmt.Sprintf("- query=%s title=%s url=%s", query, strings.TrimSpace(result.Title), strings.TrimSpace(result.URL)))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func expertiseMemoryTags(plan ExpertiseResearchPlan, query string, result websearch.Result) []string {
	tags := []string{"expertise", "expertise:" + plan.Subject, "query:" + query}
	for _, topic := range plan.AdjacentTopics {
		tags = append(tags, "related:"+topic)
	}
	if host := hostTagFromURL(result.URL); host != "" {
		tags = append(tags, "host:"+host)
	}
	return cleanMemoryTags(tags)
}

func expertiseOmnibusTags(plan ExpertiseResearchPlan) []string {
	tags := []string{"expertise", "expertise-omnibus", "expertise:" + plan.Subject}
	for _, topic := range plan.AdjacentTopics {
		tags = append(tags, "related:"+topic)
	}
	return cleanMemoryTags(tags)
}

func relatedExpertiseTagsFromRecords(records []MemoryRecord) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, record := range records {
		for _, tag := range record.Tags {
			if !strings.HasPrefix(tag, "related:") {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

func appendUniqueMemoryRecords(base []MemoryRecord, extra ...MemoryRecord) []MemoryRecord {
	seen := map[int64]struct{}{}
	for _, record := range base {
		seen[record.ID] = struct{}{}
	}
	for _, record := range extra {
		if _, ok := seen[record.ID]; ok {
			continue
		}
		seen[record.ID] = struct{}{}
		base = append(base, record)
	}
	return base
}

func summarizeExpertiseRecall(subject, question string, direct, related []MemoryRecord) string {
	lines := []string{
		"Answered from expertise memory.",
		"subject: " + subject,
		"question: " + question,
	}
	for _, record := range appendUniqueMemoryRecords(append([]MemoryRecord{}, direct...), related...) {
		lines = append(lines, "- "+firstContentLine(record.Content))
	}
	return strings.Join(lines, "\n")
}

func firstContentLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
