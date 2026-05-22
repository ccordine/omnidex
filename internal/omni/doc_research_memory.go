package omni

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultDocResearchMemoryAgent = "omni_doc_research_manager"
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
	Brief        DocumentationAuthorityBrief
	Memories     []MemoryRecord
	UsedMemory   bool
	NeedsScrape  bool
	MemorySource string
}

type DocumentationAuthorityBrief struct {
	Question       string                     `json:"question"`
	Role           string                     `json:"role"`
	GettingStarted []string                   `json:"getting_started,omitempty"`
	Conventions    []string                   `json:"conventions,omitempty"`
	Locations      []string                   `json:"locations,omitempty"`
	APIs           []string                   `json:"apis,omitempty"`
	Examples       []string                   `json:"examples,omitempty"`
	Risks          []string                   `json:"risks,omitempty"`
	Sources        []DocumentationSourceBrief `json:"sources,omitempty"`
	NeedsResearch  bool                       `json:"needs_research,omitempty"`
}

type DocumentationSourceBrief struct {
	Name     string `json:"name,omitempty"`
	URL      string `json:"url,omitempty"`
	Location string `json:"location,omitempty"`
	Excerpt  string `json:"excerpt,omitempty"`
}

type DocumentationResearchTarget struct {
	Sources []WebDocSource
	Queries []string
	Tags    []string
}

func InferDocumentationResearchTarget(question string) DocumentationResearchTarget {
	lower := strings.ToLower(question)
	target := DocumentationResearchTarget{}
	add := func(name, url string) {
		target.Sources = append(target.Sources, WebDocSource{Name: name, URL: url})
	}
	addQuery := func(values ...string) {
		target.Queries = append(target.Queries, values...)
	}
	addTags := func(values ...string) {
		target.Tags = append(target.Tags, values...)
	}

	switch {
	case strings.Contains(lower, "zig"):
		add("zig-getting-started", "https://ziglang.org/learn/getting-started/")
		add("zig-language-reference", "https://ziglang.org/documentation/master/")
		addQuery("Run Hello World", "zig init", "zig build run", "std.debug.print", "zig build-exe", "pub fn main")
		addTags("zig", "host:ziglang.org")
	case strings.Contains(lower, "go lang") || strings.Contains(lower, "golang") || strings.Contains(lower, " go ") || strings.HasPrefix(lower, "go "):
		add("go-tutorial-create-module", "https://go.dev/doc/tutorial/create-module")
		add("go-effective", "https://go.dev/doc/effective_go")
		addQuery("create a module", "go mod init", "func main", "go test", "go build")
		addTags("go", "host:go.dev")
	case strings.Contains(lower, "rust"):
		add("rust-book-hello-world", "https://doc.rust-lang.org/book/ch01-02-hello-world.html")
		add("rust-book-cargo", "https://doc.rust-lang.org/book/ch01-03-hello-cargo.html")
		addQuery("Hello, world", "cargo new", "fn main", "cargo run", "cargo test")
		addTags("rust", "host:doc.rust-lang.org")
	case strings.Contains(lower, "react") || strings.Contains(lower, "vite"):
		add("react-start-a-new-project", "https://react.dev/learn/start-a-new-react-project")
		add("vite-guide", "https://vite.dev/guide/")
		addQuery("Start a New React Project", "Vite", "create vite", "npm run build")
		addTags("react", "vite", "host:react.dev", "host:vite.dev")
	case strings.Contains(lower, "docker"):
		add("dockerfile-reference", "https://docs.docker.com/reference/dockerfile/")
		add("docker-build-guide", "https://docs.docker.com/build/concepts/dockerfile/")
		addQuery("Dockerfile", "FROM", "COPY", "RUN", "docker build", "docker run")
		addTags("docker", "host:docs.docker.com")
	}

	target.Sources = dedupeWebDocSources(target.Sources)
	target.Queries = dedupeStrings(append(target.Queries, BuildDocSearchQueries(question)...))
	target.Tags = cleanMemoryTags(target.Tags)
	return target
}

func dedupeWebDocSources(sources []WebDocSource) []WebDocSource {
	seen := map[string]struct{}{}
	out := make([]WebDocSource, 0, len(sources))
	for _, source := range sources {
		source.Name = strings.TrimSpace(source.Name)
		source.URL = strings.TrimSpace(source.URL)
		if source.URL == "" {
			continue
		}
		key := strings.ToLower(source.URL)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	return out
}

func webDocSourceURLs(sources []WebDocSource) []string {
	urls := make([]string, 0, len(sources))
	for _, source := range sources {
		if strings.TrimSpace(source.URL) != "" {
			urls = append(urls, source.URL)
		}
	}
	return urls
}

func docResearchHitsAsMemories(question string, hits []WebDocHit) []MemoryRecord {
	memories := make([]MemoryRecord, 0, len(hits))
	for _, hit := range hits {
		memories = append(memories, MemoryRecord{
			AgentID: defaultDocResearchMemoryAgent,
			Kind:    defaultDocResearchMemoryKind,
			Content: formatDocResearchMemoryContent(question, hit),
			Tags:    docResearchMemoryTags(question, hit, nil),
		})
	}
	return memories
}

func storeDocResearchHits(ctx context.Context, memory *PGMemoryStore, question string, hits []WebDocHit, tags []string) error {
	if memory == nil {
		return fmt.Errorf("memory store is required")
	}
	if err := memory.EnsureSchema(ctx); err != nil {
		return err
	}
	for _, hit := range hits {
		if _, err := memory.AddMemory(ctx, defaultDocResearchMemoryAgent, defaultDocResearchMemoryKind, formatDocResearchMemoryContent(question, hit), docResearchMemoryTags(question, hit, tags)); err != nil {
			return err
		}
	}
	return nil
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
		return DocMemoryAnswerResult{
			NeedsScrape: true,
			Brief: DocumentationAuthorityBrief{
				Question:      question,
				Role:          "documentation_specialist",
				NeedsResearch: true,
				Risks:         []string{"No matching documentation memory was found; scrape authoritative docs before giving implementation guidance."},
			},
		}, nil
	}
	brief := BuildDocumentationAuthorityBrief(question, memories)
	answer := FormatDocumentationAuthorityBrief(brief)
	return DocMemoryAnswerResult{
		Answer:       answer,
		Brief:        brief,
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
	return FormatDocumentationAuthorityBrief(BuildDocumentationAuthorityBrief(question, memories))
}

func BuildDocumentationAuthorityBrief(question string, memories []MemoryRecord) DocumentationAuthorityBrief {
	brief := DocumentationAuthorityBrief{
		Question: strings.TrimSpace(question),
		Role:     "documentation_specialist",
	}
	for _, memory := range memories {
		source := extractMemoryField(memory.Content, "source_name")
		url := extractMemoryField(memory.Content, "url")
		location := extractMemoryField(memory.Content, "location")
		excerpt := extractMemoryExcerpt(memory.Content)
		brief.Sources = append(brief.Sources, DocumentationSourceBrief{
			Name:     source,
			URL:      url,
			Location: location,
			Excerpt:  excerpt,
		})
		for _, sentence := range docExcerptSentences(excerpt) {
			classifyDocumentationGuidanceSentence(&brief, sentence)
		}
	}
	brief.GettingStarted = dedupeStrings(brief.GettingStarted)
	brief.Conventions = dedupeStrings(brief.Conventions)
	brief.Locations = dedupeStrings(brief.Locations)
	brief.APIs = dedupeStrings(brief.APIs)
	brief.Examples = dedupeStrings(brief.Examples)
	brief.Risks = dedupeStrings(brief.Risks)
	return brief
}

func FormatDocumentationAuthorityBrief(brief DocumentationAuthorityBrief) string {
	lines := []string{
		"Documentation authority brief",
		"role: " + firstNonEmpty(brief.Role, "documentation_specialist"),
		"memory_source: pgsql_memory",
		"question: " + strings.TrimSpace(brief.Question),
	}
	appendSection := func(label string, values []string) {
		if len(values) == 0 {
			return
		}
		lines = append(lines, label+":")
		for _, value := range values {
			lines = append(lines, "- "+value)
		}
	}
	appendSection("getting_started", brief.GettingStarted)
	appendSection("conventions", brief.Conventions)
	appendSection("locations", brief.Locations)
	appendSection("apis", brief.APIs)
	appendSection("examples", brief.Examples)
	appendSection("risks", brief.Risks)
	if len(brief.Sources) > 0 {
		lines = append(lines, "sources:")
		for _, source := range brief.Sources {
			sourceLine := strings.TrimSpace(strings.Join([]string{source.Name, source.URL, source.Location}, " "))
			if sourceLine != "" {
				lines = append(lines, "- "+sourceLine)
			}
			if source.Excerpt != "" {
				lines = append(lines, "  detail: "+source.Excerpt)
			}
		}
	}
	if brief.NeedsResearch {
		lines = append(lines, "needs_research: true")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func docExcerptSentences(excerpt string) []string {
	excerpt = strings.TrimSpace(excerpt)
	if excerpt == "" {
		return nil
	}
	parts := webDocSentenceEnd.Split(excerpt, -1)
	out := []string{}
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return []string{excerpt}
	}
	return out
}

func classifyDocumentationGuidanceSentence(brief *DocumentationAuthorityBrief, sentence string) {
	lower := strings.ToLower(sentence)
	switch {
	case strings.Contains(lower, "deprecated") || strings.Contains(lower, "avoid") || strings.Contains(lower, "warning") || strings.Contains(lower, "security") || strings.Contains(lower, "breaking"):
		brief.Risks = append(brief.Risks, sentence)
	case strings.Contains(lower, "file") || strings.Contains(lower, "directory") || strings.Contains(lower, "folder") || strings.Contains(lower, "path") || strings.Contains(lower, "src/") || strings.Contains(lower, "app/"):
		brief.Locations = append(brief.Locations, sentence)
	case strings.Contains(lower, "api") || strings.Contains(lower, "function") || strings.Contains(lower, "method") || strings.Contains(lower, "class") || strings.Contains(lower, "component") || strings.Contains(lower, "hook"):
		brief.APIs = append(brief.APIs, sentence)
	case strings.Contains(lower, "example") || strings.Contains(lower, "for example") || strings.Contains(lower, "usage"):
		brief.Examples = append(brief.Examples, sentence)
	case strings.Contains(lower, "install") || strings.Contains(lower, "create") || strings.Contains(lower, "start") || strings.Contains(lower, "setup") || strings.Contains(lower, "configure"):
		brief.GettingStarted = append(brief.GettingStarted, sentence)
	case strings.Contains(lower, "convention") || strings.Contains(lower, "recommend") || strings.Contains(lower, "should") || strings.Contains(lower, "must") || strings.Contains(lower, "idiomatic"):
		brief.Conventions = append(brief.Conventions, sentence)
	default:
		brief.Conventions = append(brief.Conventions, sentence)
	}
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
