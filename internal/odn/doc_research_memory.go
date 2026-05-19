package odn

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultDocResearchMemoryAgent = "odn_doc_research_manager"
	defaultDocResearchMemoryKind  = "documentation_research"
)

type DocResearchMemoryConfig struct {
	AgentID string
	Kind    string
	Tags    []string
}

type DocResearchMemoryResult struct {
	Research       WebDocResearchResult
	StoredMemories []MemoryRecord
	StoredCount    int
}

type DocMemoryAnswerResult struct {
	Answer       string
	Memories     []MemoryRecord
	UsedMemory   bool
	NeedsScrape  bool
	MemorySource string
}

func ResearchWebDocsToMemory(ctx context.Context, question string, sources []WebDocSource, queries []string, memory *PGMemoryStore, researchCfg WebDocResearchConfig, memoryCfg DocResearchMemoryConfig) (DocResearchMemoryResult, error) {
	if memory == nil {
		return DocResearchMemoryResult{}, fmt.Errorf("memory store is required")
	}
	memoryCfg = normalizeDocResearchMemoryConfig(memoryCfg)

	research, err := ResearchWebDocs(ctx, question, sources, queries, researchCfg)
	if err != nil {
		return DocResearchMemoryResult{}, err
	}
	out := DocResearchMemoryResult{Research: research}
	if err := memory.EnsureSchema(ctx); err != nil {
		return out, err
	}
	for _, hit := range research.Hits {
		content := formatDocResearchMemoryContent(question, hit)
		if strings.TrimSpace(content) == "" {
			continue
		}
		tags := docResearchMemoryTags(question, hit, memoryCfg.Tags)
		record, err := memory.AddMemory(ctx, memoryCfg.AgentID, memoryCfg.Kind, content, tags)
		if err != nil {
			return out, err
		}
		out.StoredMemories = append(out.StoredMemories, record)
		out.StoredCount++
	}
	return out, nil
}

func AnswerDocumentationQuestionFromMemory(ctx context.Context, question string, memory *PGMemoryStore, tags []string, limit int) (DocMemoryAnswerResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return DocMemoryAnswerResult{}, fmt.Errorf("question is required")
	}
	if memory == nil {
		return DocMemoryAnswerResult{}, fmt.Errorf("memory store is required")
	}
	queries := BuildDocSearchQueries(question)
	searchQuery := question
	if len(queries) > 0 {
		searchQuery = queries[0]
	}
	searchTags := cleanMemoryTags(tags)
	if len(searchTags) == 0 {
		searchTags = []string{"documentation"}
	}
	memories, err := memory.SearchMemory(ctx, searchQuery, searchTags, limit)
	if err != nil {
		return DocMemoryAnswerResult{}, err
	}
	if len(memories) == 0 && len(searchTags) > 0 {
		memories, err = memory.SearchMemory(ctx, "", searchTags, limit)
		if err != nil {
			return DocMemoryAnswerResult{}, err
		}
	}
	if len(memories) == 0 {
		return DocMemoryAnswerResult{NeedsScrape: true}, nil
	}
	answer := summarizeDocMemories(question, memories)
	return DocMemoryAnswerResult{
		Answer:       answer,
		Memories:     memories,
		UsedMemory:   true,
		MemorySource: "pgsql_memory",
	}, nil
}

func formatDocResearchMemoryContent(question string, hit WebDocHit) string {
	return strings.TrimSpace(strings.Join([]string{
		"DOC_RESEARCH_MEMORY",
		"question: " + strings.TrimSpace(question),
		"source_name: " + strings.TrimSpace(hit.Source.Name),
		"url: " + strings.TrimSpace(hit.Source.URL),
		"query: " + strings.TrimSpace(hit.Query),
		fmt.Sprintf("location: line=%d column=%d start_offset=%d end_offset=%d", hit.Line, hit.Column, hit.StartOffset, hit.EndOffset),
		"excerpt:",
		strings.TrimSpace(hit.Excerpt),
	}, "\n"))
}

func docResearchMemoryTags(question string, hit WebDocHit, extra []string) []string {
	tags := []string{
		"documentation",
		"doc-research",
		"source:" + hit.Source.Name,
		"query:" + question,
	}
	if host := hostTagFromURL(hit.Source.URL); host != "" {
		tags = append(tags, "host:"+host)
	}
	tags = append(tags, extra...)
	for _, token := range BuildDocSearchQueries(question) {
		tags = append(tags, "term:"+token)
	}
	return cleanMemoryTags(tags)
}

func normalizeDocResearchMemoryConfig(cfg DocResearchMemoryConfig) DocResearchMemoryConfig {
	if strings.TrimSpace(cfg.AgentID) == "" {
		cfg.AgentID = defaultDocResearchMemoryAgent
	}
	if strings.TrimSpace(cfg.Kind) == "" {
		cfg.Kind = defaultDocResearchMemoryKind
	}
	return cfg
}

func summarizeDocMemories(question string, memories []MemoryRecord) string {
	lines := []string{
		"Answered from pgsql_memory.",
		"question: " + strings.TrimSpace(question),
	}
	for _, memory := range memories {
		source := extractMemoryField(memory.Content, "source_name")
		url := extractMemoryField(memory.Content, "url")
		location := extractMemoryField(memory.Content, "location")
		excerpt := extractMemoryExcerpt(memory.Content)
		if source != "" || url != "" {
			lines = append(lines, "source: "+strings.TrimSpace(strings.Join([]string{source, url}, " ")))
		}
		if location != "" {
			lines = append(lines, "location: "+location)
		}
		if excerpt != "" {
			lines = append(lines, "detail: "+excerpt)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func extractMemoryField(content, field string) string {
	prefix := field + ":"
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func extractMemoryExcerpt(content string) string {
	marker := "excerpt:"
	index := strings.Index(content, marker)
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(content[index+len(marker):])
}
