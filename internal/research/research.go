package research

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	urlpkg "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ingest"
)

const (
	DefaultSourcePrefix = "research"
	DefaultKind         = "reference"
)

var htmlTagRE = regexp.MustCompile(`(?is)<[^>]+>`)
var whitespaceRE = regexp.MustCompile(`\s+`)
var sourceTokenRE = regexp.MustCompile(`[^a-z0-9]+`)

type Document struct {
	Section string `json:"section"`
	Content string `json:"content"`
}

type PreparedChunk struct {
	Source  string   `json:"source"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type PrepareOptions struct {
	Topic        string
	Slug         string
	SourcePrefix string
	Tags         []string
	ChunkSize    int
	Overlap      int
	MaxChunks    int
}

func SearchQuery(topic string) string {
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

func BuildInstruction(topic string, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	today := now.Format("2006-01-02")
	if topicLooksTechnical(topic) {
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

func OfficialSourceURLs(topic string) []string {
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

func FetchOfficialDocuments(ctx context.Context, topic string) ([]Document, []string, error) {
	urls := OfficialSourceURLs(topic)
	if len(urls) == 0 {
		return nil, nil, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	docs := make([]Document, 0, len(urls))
	warnings := []string{}
	for _, rawURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return docs, warnings, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; omnidex-research/1.0)")
		resp, err := client.Do(req)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("official source fetch failed %s: %v", rawURL, err))
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("official source read failed %s: %v", rawURL, readErr))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			warnings = append(warnings, fmt.Sprintf("official source returned status=%d url=%s", resp.StatusCode, rawURL))
			continue
		}
		text := HTMLToText(string(body))
		if strings.TrimSpace(text) == "" {
			continue
		}
		docs = append(docs, Document{
			Section: "official-source",
			Content: BuildContentBlock(rawURL, 0, "official_source", "url: "+rawURL+"\ncontent:\n"+text),
		})
	}
	return docs, warnings, nil
}

func PrepareChunks(documents []Document, opts PrepareOptions) []PreparedChunk {
	slug := SanitizeToken(opts.Slug)
	if slug == "" {
		slug = SanitizeToken(opts.Topic)
	}
	if slug == "" {
		slug = fmt.Sprintf("topic-%d", time.Now().Unix())
	}
	prefix := strings.TrimSpace(opts.SourcePrefix)
	if prefix == "" {
		prefix = DefaultSourcePrefix
	}
	maxChunks := opts.MaxChunks
	if maxChunks <= 0 {
		maxChunks = 24
	}
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 1800
	}
	overlap := opts.Overlap
	if overlap < 0 {
		overlap = 0
	}

	type preparedDoc struct {
		document    Document
		sectionSlug string
		sourceSlug  string
		chunks      []string
	}
	prepared := make([]preparedDoc, 0, len(documents))
	for i, doc := range documents {
		chunks := ingest.ChunkText(doc.Content, chunkSize, overlap)
		if len(chunks) == 0 {
			continue
		}
		sectionSlug := SanitizeToken(doc.Section)
		if sectionSlug == "" {
			sectionSlug = "section"
		}
		prepared = append(prepared, preparedDoc{
			document:    doc,
			sectionSlug: sectionSlug,
			sourceSlug:  DocumentSourceSlug(doc, i),
			chunks:      chunks,
		})
	}

	out := make([]PreparedChunk, 0, maxChunks)
	baseTags := MergeTags(opts.Tags, InferTags(opts.Topic, slug))
	for round := 0; len(out) < maxChunks; round++ {
		added := false
		for _, doc := range prepared {
			if len(out) >= maxChunks {
				break
			}
			if round >= len(doc.chunks) {
				continue
			}
			source := fmt.Sprintf("%s:%s:%s:%s#%03d", prefix, slug, doc.sectionSlug, doc.sourceSlug, round+1)
			out = append(out, PreparedChunk{
				Source:  source,
				Content: PrefixChunkMetadata(doc.document, doc.chunks[round]),
				Tags:    baseTags,
			})
			added = true
		}
		if !added {
			break
		}
	}
	return out
}

func HTMLToText(body string) string {
	text := htmlTagRE.ReplaceAllString(body, " ")
	text = html.UnescapeString(text)
	text = whitespaceRE.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func BuildContentBlock(topic string, jobID int64, section, content string) string {
	return strings.Join([]string{
		"Research memory",
		"topic=" + topic,
		"section=" + strings.TrimSpace(section),
		fmt.Sprintf("job_id=%d", jobID),
		"captured_at=" + time.Now().UTC().Format(time.RFC3339),
		"content:",
		strings.TrimSpace(content),
	}, "\n")
}

func PrefixChunkMetadata(doc Document, chunk string) string {
	cleanChunk := strings.TrimSpace(chunk)
	if cleanChunk == "" {
		return ""
	}
	lines := []string{
		"Research chunk metadata:",
		"section=" + safeValue(strings.TrimSpace(doc.Section), "section"),
	}
	if url := DocumentURL(doc.Content); url != "" {
		lines = append(lines, "source_url="+url)
	}
	return strings.Join(lines, "\n") + "\n\n" + cleanChunk
}

func DocumentSourceSlug(doc Document, index int) string {
	if url := DocumentURL(doc.Content); url != "" {
		if parsed, err := urlpkg.Parse(url); err == nil {
			parts := []string{parsed.Host}
			for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
				if clean := strings.TrimSpace(part); clean != "" {
					parts = append(parts, clean)
				}
			}
			if slug := SanitizeToken(strings.Join(parts, "-")); slug != "" {
				return slug
			}
		}
		if slug := SanitizeToken(url); slug != "" {
			return slug
		}
	}
	if slug := SanitizeToken(doc.Section); slug != "" {
		return fmt.Sprintf("%s-%02d", slug, index+1)
	}
	return fmt.Sprintf("doc-%02d", index+1)
}

func DocumentURL(content string) string {
	for _, line := range strings.Split(content, "\n") {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(clean), "url:") {
			return strings.TrimSpace(clean[len("url:"):])
		}
	}
	return ""
}

func BuildDossier(topic string, jobID int64, requestedAt time.Time, documents []Document, tags []string, sourcePrefix string, storedChunks int) string {
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

func InferTags(topic, slug string) []string {
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

func SanitizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = sourceTokenRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-")
	}
	return value
}

func MergeTags(parts ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 16)
	for _, list := range parts {
		for _, raw := range list {
			tag := strings.ToLower(strings.TrimSpace(raw))
			if tag == "" {
				continue
			}
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}

func NormalizeReasoning(value string) string {
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

func topicLooksTechnical(topic string) bool {
	lower := strings.ToLower(topic)
	needles := []string{"api", "docker", "go lang", "golang", "javascript", "node", "php", "postgres", "postgresql", "pgsql", "react", "rust", "software", "typescript", "vite", "zig"}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func normalizeForMatch(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("_", " ", "-", " ", "/", " ", ".", " ", ",", " ", ":", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
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

func safeValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
