package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/ingest"
	"github.com/gryph/omnidex/internal/model"
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
