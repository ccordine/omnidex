package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

const defaultResearchManifestPath = ".omni/research-index.json"
const defaultResearchArchiveRoot = ".omni/research"

var researchHTMLTagRE = regexp.MustCompile(`(?is)<[^>]+>`)
var researchWhitespaceRE = regexp.MustCompile(`\s+`)

type researchIndex struct {
	Entries map[string]researchEntry `json:"entries"`
}

type researchEntry struct {
	Topic            string   `json:"topic"`
	Slug             string   `json:"slug"`
	SourcePrefix     string   `json:"source_prefix"`
	LastResearchedAt string   `json:"last_researched_at"`
	LastJobID        int64    `json:"last_job_id"`
	StoredChunks     int      `json:"stored_chunks"`
	Tags             []string `json:"tags,omitempty"`
	FullTextPath     string   `json:"full_text_path,omitempty"`
}

type researchDocument struct {
	Section string
	Content string
}

type researchPreparedDocument struct {
	Document    researchDocument
	SectionSlug string
	SourceSlug  string
	Chunks      []string
}

type researchMemoryStore interface {
	Store(ctx context.Context, source, kind, content string, tags []string) error
}

type apiResearchMemoryStore struct {
	client *client.Client
}

func (s apiResearchMemoryStore) Store(ctx context.Context, source, kind, content string, tags []string) error {
	_, err := s.client.AddMemory(ctx, source, kind, content, tags)
	return err
}

type directDBResearchMemoryStore struct {
	repo     *queue.Repository
	embedder interface {
		Embedding(context.Context, string) ([]float64, error)
	}
}

func (s directDBResearchMemoryStore) Store(ctx context.Context, source, kind, content string, tags []string) error {
	var embedding []float64
	if s.embedder != nil {
		if vector, err := s.embedder.Embedding(ctx, content); err == nil {
			embedding = vector
		}
	}
	_, err := s.repo.AddMemoryChunk(ctx, source, kind, content, tags, embedding)
	return err
}

func runResearch(c *client.Client, args []string) {
	fs := flag.NewFlagSet("research", flag.ExitOnError)
	sourcePrefix := fs.String("source", "research", "memory source prefix")
	kind := fs.String("kind", model.MemoryKindReference, "memory kind")
	tags := fs.String("tags", "", "comma-separated extra tags")
	refreshDays := fs.Int("refresh-days", 14, "skip re-research if prior run is newer than this many days (0 disables freshness check)")
	force := fs.Bool("force", false, "force refresh regardless of freshness")
	includeWebContext := fs.Bool("include-web-context", true, "store web search context alongside the synthesized report")
	includeAnalyzeContext := fs.Bool("include-analyze-context", true, "store analyze step context alongside the synthesized report")
	includeOfficialSources := fs.Bool("include-official-sources", true, "fetch and store direct official source pages for recognized technical topics")
	chunkSize := fs.Int("chunk-size", 1800, "memory chunk size in characters")
	overlap := fs.Int("overlap", 220, "memory chunk overlap in characters")
	maxChunks := fs.Int("max-chunks", 24, "max number of chunks stored for a research run")
	reasoningLevel := fs.String("reasoning", "deep", "thinking level for research job: auto|fast|deep")
	sessionID := fs.String("session", "", "optional session/thread identifier")
	manifestPath := fs.String("manifest", defaultResearchManifestPath, "path to local research freshness index")
	archiveRoot := fs.String("archive-root", defaultResearchArchiveRoot, "directory for full-text research dossiers")
	storeMode := fs.String("store", "api", "memory storage mode: api|direct-db")
	databaseURL := fs.String("database-url", getenv("DATABASE_URL", ""), "Postgres URL for --store direct-db")
	embeddingBaseURL := fs.String("embedding-base-url", getenv("OLLAMA_BASE_URL", "http://localhost:11434"), "Ollama base URL for --store direct-db embeddings")
	embeddingModel := fs.String("embedding-model", getenv("EMBEDDING_MODEL", "nomic-embed-text"), "embedding model for --store direct-db")
	interval := fs.Duration("interval", 2*time.Second, "poll interval while waiting for the research job")
	timeout := fs.Duration("timeout", 20*time.Minute, "max time to wait for research completion")
	_ = fs.Parse(args)

	topic := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if topic == "" {
		die("research requires a topic")
	}

	normalizedReasoning := normalizeResearchReasoning(*reasoningLevel)
	if normalizedReasoning == "" {
		die("invalid --reasoning value (use auto|fast|deep)")
	}
	if normalizeResearchStoreMode(*storeMode) == "" {
		die("invalid --store value (use api|direct-db)")
	}

	slug := sanitizeMemorySourceToken(topic)
	if slug == "" {
		slug = fmt.Sprintf("topic-%d", time.Now().Unix())
	}

	manifest, err := loadResearchIndex(*manifestPath)
	if err != nil {
		die(fmt.Sprintf("failed loading research manifest: %v", err))
	}

	now := time.Now()
	if !*force {
		if entry, ok := manifest.Entries[slug]; ok {
			fresh, age := researchEntryFresh(entry, now, *refreshDays)
			if fresh {
				fmt.Printf("research for %q is fresh (last=%s age=%s). Use --force to refresh now.\n", topic, entry.LastResearchedAt, age.Round(time.Minute))
				return
			}
		}
	}

	instruction := buildResearchInstruction(topic, now)
	metadata := map[string]any{
		"web_search":              "force",
		"search_query":            researchSearchQuery(topic),
		"workspace_scan":          "off",
		"allow_missing_tools":     true,
		"reasoning_level":         normalizedReasoning,
		"autonomy_mode":           "on",
		"approval_mode":           "off",
		"verification_mode":       "off",
		"verification_iterations": 1,
		"research_topic":          topic,
		"research_slug":           slug,
		"research_requested_at":   now.UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(*sessionID) != "" {
		metadata["session_id"] = strings.TrimSpace(*sessionID)
	}

	cwd := ""
	if dir, err := os.Getwd(); err == nil && strings.TrimSpace(dir) != "" {
		cwd = strings.TrimSpace(dir)
		metadata["client_cwd"] = cwd
	}
	applyHostEnvironmentMetadata(metadata, discoverHostEnvironmentSnapshot(cwd))
	applyHostTemporalMetadata(metadata, now)

	fmt.Printf("starting research job for topic=%q\n", topic)
	job, err := c.Enqueue(context.Background(), instruction, model.PipelineAssistant, metadata)
	if err != nil {
		die(err.Error())
	}

	details, err := awaitResearchJob(c, job.ID, *interval, *timeout)
	if err != nil {
		die(err.Error())
	}

	var officialDocs []researchDocument
	if *includeOfficialSources {
		fetched, err := fetchOfficialResearchDocuments(context.Background(), topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: failed fetching official research sources: %v\n", err)
		}
		officialDocs = fetched
	}

	switch details.Job.Status {
	case model.JobStatusWaiting:
		question := latestContextValue(details.Contexts, "input_question")
		if strings.TrimSpace(question) == "" {
			question = "additional input required"
		}
		die(fmt.Sprintf("research job %d needs input: %s (answer with: omni feedback %d \"...\")", job.ID, question, job.ID))
	case model.JobStatusFailed:
		if len(officialDocs) == 0 {
			die(fmt.Sprintf("research job %d failed: %s", job.ID, safeValue(strings.TrimSpace(details.Job.Error), "unknown error")))
		}
		fmt.Fprintf(os.Stderr, "warn: research job %d failed (%s); continuing with direct official sources\n", job.ID, safeValue(strings.TrimSpace(details.Job.Error), "unknown error"))
	case model.JobStatusCanceled:
		die(fmt.Sprintf("research job %d was canceled", job.ID))
	case model.JobStatusCompleted:
	default:
		die(fmt.Sprintf("research job %d ended in unexpected status=%s", job.ID, details.Job.Status))
	}

	documents := collectResearchDocuments(topic, details, *includeWebContext, *includeAnalyzeContext)
	documents = append(documents, officialDocs...)
	if len(documents) == 0 {
		die("research completed, but no ingestible research content was produced")
	}

	prefix := strings.TrimSpace(*sourcePrefix)
	if prefix == "" {
		prefix = "research"
	}
	baseTags := mergeTags(splitTags(*tags), inferResearchTags(topic, slug))
	store, closeStore, err := openResearchMemoryStore(context.Background(), normalizeResearchStoreMode(*storeMode), c, *databaseURL, *embeddingBaseURL, *embeddingModel)
	if err != nil {
		die(err.Error())
	}
	defer closeStore()
	stored := 0
	maxAllowed := *maxChunks
	if maxAllowed <= 0 {
		maxAllowed = 24
	}

	preparedDocs := make([]researchPreparedDocument, 0, len(documents))
	for docIndex, doc := range documents {
		chunks := ingest.ChunkText(doc.Content, *chunkSize, *overlap)
		if len(chunks) == 0 {
			continue
		}

		sectionSlug := sanitizeMemorySourceToken(doc.Section)
		if sectionSlug == "" {
			sectionSlug = "section"
		}
		sourceSlug := researchDocumentSourceSlug(doc, docIndex)
		preparedDocs = append(preparedDocs, researchPreparedDocument{Document: doc, SectionSlug: sectionSlug, SourceSlug: sourceSlug, Chunks: chunks})
	}

	for round := 0; stored < maxAllowed; round++ {
		added := false
		for _, prepared := range preparedDocs {
			if stored >= maxAllowed {
				break
			}
			if round >= len(prepared.Chunks) {
				continue
			}
			source := fmt.Sprintf("%s:%s:%s:%s#%03d", prefix, slug, prepared.SectionSlug, prepared.SourceSlug, round+1)
			chunk := prefixResearchChunkMetadata(prepared.Document, prepared.Chunks[round])
			if err := store.Store(context.Background(), source, *kind, chunk, baseTags); err != nil {
				fmt.Fprintf(os.Stderr, "warn: failed storing research chunk %s: %v\n", source, err)
				continue
			}
			stored++
			added = true
		}
		if !added {
			break
		}
	}

	if stored == 0 {
		die("research completed, but no memory chunks were stored")
	}

	dossierPath, err := writeResearchDossier(*archiveRoot, slug, topic, job.ID, now, documents, baseTags, prefix, stored)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: failed writing full-text research dossier: %v\n", err)
	}

	manifest.Entries[slug] = researchEntry{
		Topic:            topic,
		Slug:             slug,
		SourcePrefix:     prefix,
		LastResearchedAt: now.UTC().Format(time.RFC3339),
		LastJobID:        job.ID,
		StoredChunks:     stored,
		Tags:             baseTags,
		FullTextPath:     dossierPath,
	}
	if err := saveResearchIndex(*manifestPath, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "warn: failed updating research manifest %s: %v\n", strings.TrimSpace(*manifestPath), err)
	}

	fmt.Printf("research complete topic=%q job=%d stored_chunks=%d tags=%s\n", topic, job.ID, stored, strings.Join(baseTags, ","))
	fmt.Printf("manifest=%s\n", strings.TrimSpace(*manifestPath))
	if strings.TrimSpace(dossierPath) != "" {
		fmt.Printf("full_text_dossier=%s\n", dossierPath)
	}
}

func awaitResearchJob(c *client.Client, jobID int64, interval, timeout time.Duration) (model.JobDetails, error) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if timeout <= 0 {
		timeout = 20 * time.Minute
	}

	deadline := time.Now().Add(timeout)
	lastStatus := ""
	for {
		details, err := c.Show(context.Background(), jobID)
		if err != nil {
			return model.JobDetails{}, err
		}

		if details.Job.Status != lastStatus {
			fmt.Printf("research job %d status=%s\n", jobID, details.Job.Status)
			lastStatus = details.Job.Status
		}

		switch details.Job.Status {
		case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled, model.JobStatusWaiting:
			return details, nil
		}

		if time.Now().After(deadline) {
			return model.JobDetails{}, fmt.Errorf("research job %d timed out after %s", jobID, timeout)
		}
		time.Sleep(interval)
	}
}

func collectResearchDocuments(topic string, details model.JobDetails, includeWebContext, includeAnalyzeContext bool) []researchDocument {
	docs := make([]researchDocument, 0, 3)
	result := strings.TrimSpace(details.Job.Result)
	if result != "" {
		docs = append(docs, researchDocument{
			Section: "report",
			Content: buildResearchContentBlock(topic, details.Job.ID, "report", result),
		})
	}

	webContext := strings.TrimSpace(latestContextValue(details.Contexts, "web_search"))
	if includeWebContext && webContext != "" && !strings.Contains(strings.ToLower(webContext), "web search skipped") {
		docs = append(docs, researchDocument{
			Section: "web-context",
			Content: buildResearchContentBlock(topic, details.Job.ID, "web_context", webContext),
		})
	}

	analyzeContext := strings.TrimSpace(latestContextValue(details.Contexts, "analyze"))
	if includeAnalyzeContext && analyzeContext != "" {
		docs = append(docs, researchDocument{
			Section: "analysis",
			Content: buildResearchContentBlock(topic, details.Job.ID, "analysis", analyzeContext),
		})
	}

	return docs
}

func fetchOfficialResearchDocuments(ctx context.Context, topic string) ([]researchDocument, error) {
	urls := officialResearchSourceURLs(topic)
	if len(urls) == 0 {
		return nil, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	docs := make([]researchDocument, 0, len(urls))
	for _, rawURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return docs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; omnidex-research/1.0)")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: official source fetch failed %s: %v\n", rawURL, err)
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "warn: official source read failed %s: %v\n", rawURL, readErr)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			fmt.Fprintf(os.Stderr, "warn: official source returned status=%d url=%s\n", resp.StatusCode, rawURL)
			continue
		}
		text := researchHTMLToText(string(body))
		if strings.TrimSpace(text) == "" {
			continue
		}
		docs = append(docs, researchDocument{
			Section: "official-source",
			Content: buildResearchContentBlock(rawURL, 0, "official_source", "url: "+rawURL+"\ncontent:\n"+text),
		})
	}
	return docs, nil
}

func officialResearchSourceURLs(topic string) []string {
	lower := strings.ToLower(topic)
	switch {
	case strings.Contains(lower, "vite"):
		return []string{
			"https://vite.dev/guide/",
			"https://vite.dev/config/",
			"https://vite.dev/guide/features.html",
			"https://vite.dev/guide/build.html",
			"https://vite.dev/guide/dep-pre-bundling.html",
			"https://vite.dev/guide/troubleshooting.html",
		}
	case strings.Contains(lower, "react"):
		return []string{
			"https://react.dev/learn",
			"https://react.dev/reference/react",
			"https://react.dev/reference/react-dom",
			"https://react.dev/blog",
			"https://vite.dev/guide/",
		}
	case strings.Contains(lower, "node.js") || strings.Contains(lower, "nodejs") || strings.Contains(lower, "node js"):
		return []string{
			"https://nodejs.org/api/",
			"https://nodejs.org/en/learn",
			"https://nodejs.org/en/learn/getting-started/introduction-to-nodejs",
			"https://nodejs.org/en/learn/asynchronous-work/event-loop-timers-and-nexttick",
			"https://nodejs.org/en/learn/getting-started/security-best-practices",
		}
	case strings.Contains(lower, "rust"):
		return []string{
			"https://doc.rust-lang.org/book/",
			"https://doc.rust-lang.org/reference/",
			"https://doc.rust-lang.org/cargo/",
			"https://doc.rust-lang.org/nomicon/",
			"https://docs.rs/tokio/latest/tokio/",
		}
	case strings.Contains(lower, "golang") || strings.Contains(lower, "go "):
		return []string{
			"https://go.dev/doc/",
			"https://go.dev/doc/effective_go",
			"https://pkg.go.dev/std",
			"https://go.dev/doc/modules/managing-dependencies",
		}
	case strings.Contains(lower, "php"):
		return []string{
			"https://www.php.net/manual/en/",
			"https://www.php.net/manual/en/language.types.declarations.php",
			"https://getcomposer.org/doc/",
			"https://www.php-fig.org/psr/",
		}
	case strings.Contains(lower, "docker"):
		return []string{
			"https://docs.docker.com/get-started/",
			"https://docs.docker.com/build/",
			"https://docs.docker.com/compose/",
			"https://docs.docker.com/build/building/best-practices/",
		}
	case strings.Contains(lower, "postgres") || strings.Contains(lower, "pgsql") || strings.Contains(lower, "postgresql"):
		return []string{
			"https://www.postgresql.org/docs/current/",
			"https://www.postgresql.org/docs/current/sql.html",
			"https://www.postgresql.org/docs/current/indexes.html",
			"https://www.postgresql.org/docs/current/performance-tips.html",
		}
	case strings.Contains(lower, "javascript") || strings.Contains(lower, "node"):
		return []string{
			"https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference",
			"https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide",
			"https://nodejs.org/api/",
			"https://nodejs.org/en/learn",
		}
	default:
		return nil
	}
}

func researchHTMLToText(body string) string {
	text := researchHTMLTagRE.ReplaceAllString(body, " ")
	text = html.UnescapeString(text)
	text = researchWhitespaceRE.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func buildResearchContentBlock(topic string, jobID int64, section, content string) string {
	lines := []string{
		"Research memory",
		"topic=" + topic,
		"section=" + strings.TrimSpace(section),
		fmt.Sprintf("job_id=%d", jobID),
		"captured_at=" + time.Now().UTC().Format(time.RFC3339),
		"content:",
		strings.TrimSpace(content),
	}
	return strings.Join(lines, "\n")
}

func prefixResearchChunkMetadata(doc researchDocument, chunk string) string {
	cleanChunk := strings.TrimSpace(chunk)
	if cleanChunk == "" {
		return ""
	}
	lines := []string{
		"Research chunk metadata:",
		"section=" + safeValue(strings.TrimSpace(doc.Section), "section"),
	}
	if url := researchDocumentURL(doc.Content); url != "" {
		lines = append(lines, "source_url="+url)
	}
	return strings.Join(lines, "\n") + "\n\n" + cleanChunk
}

func researchDocumentSourceSlug(doc researchDocument, index int) string {
	if url := researchDocumentURL(doc.Content); url != "" {
		if parsed, err := urlpkg.Parse(url); err == nil {
			parts := []string{parsed.Host}
			for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
				if clean := strings.TrimSpace(part); clean != "" {
					parts = append(parts, clean)
				}
			}
			if slug := sanitizeMemorySourceToken(strings.Join(parts, "-")); slug != "" {
				return slug
			}
		}
		if slug := sanitizeMemorySourceToken(url); slug != "" {
			return slug
		}
	}
	if slug := sanitizeMemorySourceToken(doc.Section); slug != "" {
		return fmt.Sprintf("%s-%02d", slug, index+1)
	}
	return fmt.Sprintf("doc-%02d", index+1)
}

func researchDocumentURL(content string) string {
	for _, line := range strings.Split(content, "\n") {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(clean), "url:") {
			return strings.TrimSpace(clean[len("url:"):])
		}
	}
	return ""
}

func writeResearchDossier(root, slug, topic string, jobID int64, requestedAt time.Time, documents []researchDocument, tags []string, sourcePrefix string, storedChunks int) (string, error) {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		return "", nil
	}
	cleanSlug := sanitizeMemorySourceToken(slug)
	if cleanSlug == "" {
		cleanSlug = fmt.Sprintf("topic-%d", jobID)
	}
	if err := os.MkdirAll(cleanRoot, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(cleanRoot, fmt.Sprintf("%s-job-%d.md", cleanSlug, jobID))
	body := buildResearchDossier(topic, jobID, requestedAt, documents, tags, sourcePrefix, storedChunks)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildResearchDossier(topic string, jobID int64, requestedAt time.Time, documents []researchDocument, tags []string, sourcePrefix string, storedChunks int) string {
	if requestedAt.IsZero() {
		requestedAt = time.Now()
	}
	lines := []string{
		"# Research Dossier",
		"",
		"topic: " + strings.TrimSpace(topic),
		fmt.Sprintf("job_id: %d", jobID),
		"captured_at: " + requestedAt.UTC().Format(time.RFC3339),
		"source_prefix: " + strings.TrimSpace(sourcePrefix),
		fmt.Sprintf("stored_memory_chunks: %d", storedChunks),
		"tags: " + strings.Join(cleanDossierTags(tags), ","),
		"",
		"## Purpose",
		"",
		"This file is the full-text reference account captured for the research run. Memory chunks are optimized for retrieval; this dossier preserves the larger text Omnidex used, including synthesized report, web context, analysis context, URLs, excerpts, and source notes when available.",
		"",
	}
	for _, doc := range documents {
		section := strings.TrimSpace(doc.Section)
		if section == "" {
			section = "section"
		}
		lines = append(lines, "## "+section, "", strings.TrimSpace(doc.Content), "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func cleanDossierTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		clean := strings.TrimSpace(tag)
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

func buildResearchInstruction(topic string, now time.Time) string {
	today := now.Format("2006-01-02")
	if researchTopicLooksTechnical(topic) {
		return strings.TrimSpace(fmt.Sprintf(`Research the topic "%s" comprehensively and produce a durable technical expertise reference.

Requirements:
1) Prefer primary/official documentation and include source URLs inline.
2) Cover the current recommended project setup, core concepts, APIs, conventions, file structure, dependency/tooling expectations, testing, debugging, deployment/build notes, and production pitfalls.
3) Include small canonical examples and explain when to use each pattern.
4) Call out outdated/deprecated guidance, version-sensitive behavior, uncertainty, or conflicting information explicitly.
5) Organize output with clear markdown headings and concise bullets that are useful to an implementation planner.
6) End with a short "Last verified" line using date %s.
`, topic, today))
	}
	return strings.TrimSpace(fmt.Sprintf(`Research the topic "%s" comprehensively and produce a durable technical reference.

Requirements:
1) Cover core overview, timeline/history, terminology/glossary, key entities, major systems/mechanics, and practical FAQs.
2) Include detailed, concrete facts and edge cases. For games/media topics, include quests/missions, items/equipment, factions/characters, locations, and in-universe language/slang.
3) Prefer primary/official sources when possible and include source URLs inline.
4) Call out uncertainty or conflicting information explicitly.
5) Organize output with clear markdown headings and concise bullet points.
6) End with a short "Last verified" line using date %s.
`, topic, today))
}

func researchTopicLooksTechnical(topic string) bool {
	lower := strings.ToLower(topic)
	needles := []string{
		"api",
		"docker",
		"go lang",
		"golang",
		"javascript",
		"node",
		"php",
		"postgres",
		"postgresql",
		"pgsql",
		"react",
		"rust",
		"software",
		"typescript",
		"zig",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func normalizeResearchStoreMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "api":
		return "api"
	case "direct-db", "direct_db", "db":
		return "direct-db"
	default:
		return ""
	}
}

func openResearchMemoryStore(ctx context.Context, mode string, apiClient *client.Client, databaseURL, embeddingBaseURL, embeddingModel string) (researchMemoryStore, func(), error) {
	switch mode {
	case "api":
		return apiResearchMemoryStore{client: apiClient}, func() {}, nil
	case "direct-db":
		cleanURL := strings.TrimSpace(databaseURL)
		if cleanURL == "" {
			return nil, func() {}, fmt.Errorf("--store direct-db requires --database-url or DATABASE_URL")
		}
		pool, err := db.Connect(ctx, cleanURL)
		if err != nil {
			return nil, func() {}, fmt.Errorf("connect research database: %w", err)
		}
		repo := queue.New(pool)
		embedder := ollama.New(embeddingBaseURL, "", embeddingModel, 2*time.Minute)
		return directDBResearchMemoryStore{repo: repo, embedder: embedder}, pool.Close, nil
	default:
		return nil, func() {}, fmt.Errorf("invalid research memory store mode %q", mode)
	}
}

func researchSearchQuery(topic string) string {
	clean := strings.TrimSpace(topic)
	if clean == "" {
		return ""
	}
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 180 {
		clean = clean[:180]
	}
	lower := strings.ToLower(clean)
	switch {
	case strings.Contains(lower, "vite"):
		return clean + " Vite official documentation guide config build plugins HMR production"
	case strings.Contains(lower, "react"):
		return clean + " React official documentation react.dev learn reference hooks components Vite"
	case strings.Contains(lower, "node.js") || strings.Contains(lower, "nodejs") || strings.Contains(lower, "node js"):
		return clean + " Node.js official documentation API learn event loop modules streams security"
	case strings.Contains(lower, "rust"):
		return clean + " official Rust documentation reference Cargo book Rustonomicon Tokio docs"
	case strings.Contains(lower, "golang") || strings.Contains(lower, "go "):
		return clean + " go.dev official documentation Effective Go standard library"
	case strings.Contains(lower, "php"):
		return clean + " php.net manual official documentation composer psr"
	case strings.Contains(lower, "docker"):
		return clean + " Docker official documentation compose buildfile best practices"
	case strings.Contains(lower, "postgres") || strings.Contains(lower, "pgsql") || strings.Contains(lower, "postgresql"):
		return clean + " PostgreSQL official documentation current manual"
	case strings.Contains(lower, "javascript") || strings.Contains(lower, "node"):
		return clean + " MDN JavaScript reference Node.js official documentation"
	default:
		return clean + " official documentation reference guide"
	}
}

func normalizeResearchReasoning(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "deep":
		return "deep"
	case "auto":
		return "auto"
	case "fast":
		return "fast"
	default:
		return ""
	}
}

func inferResearchTags(topic, slug string) []string {
	out := []string{"research", "reference", "knowledge-base"}
	if clean := strings.TrimSpace(slug); clean != "" {
		out = append(out, "topic-"+clean)
	}
	for _, token := range strings.Fields(normalizeForMatch(topic)) {
		if len(token) < 3 {
			continue
		}
		out = append(out, token)
	}
	out = append(out, "as-of-"+time.Now().Format("2006"))
	return out
}

func loadResearchIndex(path string) (researchIndex, error) {
	index := researchIndex{Entries: map[string]researchEntry{}}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return index, nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, nil
		}
		return index, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return index, nil
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, err
	}
	if index.Entries == nil {
		index.Entries = map[string]researchEntry{}
	}
	return index, nil
}

func saveResearchIndex(path string, index researchIndex) error {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil
	}
	if index.Entries == nil {
		index.Entries = map[string]researchEntry{}
	}

	dir := filepath.Dir(cleanPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(cleanPath, encoded, 0o644)
}

func researchEntryFresh(entry researchEntry, now time.Time, refreshDays int) (bool, time.Duration) {
	if refreshDays <= 0 {
		return false, 0
	}
	timestamp := strings.TrimSpace(entry.LastResearchedAt)
	if timestamp == "" {
		return false, 0
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false, 0
	}
	age := now.Sub(parsed)
	if age < 0 {
		age = 0
	}
	freshWindow := time.Duration(refreshDays) * 24 * time.Hour
	return age < freshWindow, age
}
