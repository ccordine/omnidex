package odn

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const defaultWebDocFetchTimeout = 25 * time.Second

var (
	scriptBlockRe      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleBlockRe       = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	metaContentRe      = regexp.MustCompile(`(?is)<meta\b[^>]*(?:name|property)=["'](?:description|og:description|twitter:description)["'][^>]*content=["']([^"']+)["'][^>]*>`)
	htmlTagRe          = regexp.MustCompile(`(?s)<[^>]+>`)
	whitespaceRe       = regexp.MustCompile(`[ \t\r\f\v]+`)
	blankLineRe        = regexp.MustCompile(`\n{3,}`)
	webDocSentenceEnd  = regexp.MustCompile(`(?m)([.!?])\s+`)
	webDocNonWordQuery = regexp.MustCompile(`[^a-zA-Z0-9_.:-]+`)
)

type WebDocSource struct {
	Name string
	URL  string
}

type WebDocResearchConfig struct {
	FetchTimeout time.Duration
	ChunkConfig  DocumentSearchConfig
	MaxBytes     int64
	MaxHits      int
}

type WebDocHit struct {
	Source      WebDocSource
	Query       string
	StartOffset int
	EndOffset   int
	Line        int
	Column      int
	Excerpt     string
}

type WebDocResearchResult struct {
	Question    string
	Sources     []WebDocSource
	Hits        []WebDocHit
	Found       bool
	SourceCount int
	ChunkCount  int
	WorkerCount int
}

func ResearchWebDocs(ctx context.Context, question string, sources []WebDocSource, queries []string, cfg WebDocResearchConfig) (WebDocResearchResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return WebDocResearchResult{}, fmt.Errorf("question cannot be empty")
	}
	if len(sources) == 0 {
		return WebDocResearchResult{}, fmt.Errorf("at least one web documentation source is required")
	}
	cfg = normalizeWebDocResearchConfig(cfg)
	if len(queries) == 0 {
		queries = BuildDocSearchQueries(question)
	}

	result := WebDocResearchResult{Question: question, Sources: sources, SourceCount: len(sources)}
	for _, source := range sources {
		raw, err := fetchWebDoc(ctx, source.URL, cfg)
		if err != nil {
			return result, fmt.Errorf("fetch %s: %w", source.URL, err)
		}
		text := HTMLToSearchableText(raw)
		sourceChunks := ChunkDocument(text, cfg.ChunkConfig)
		result.ChunkCount += len(sourceChunks)
		result.WorkerCount += len(sourceChunks)
		for _, query := range queries {
			query = strings.TrimSpace(query)
			if query == "" {
				continue
			}
			searchResult, err := SearchLargeDocument(text, query, cfg.ChunkConfig)
			if err != nil {
				continue
			}
			for _, hit := range searchResult.Hits {
				result.Hits = append(result.Hits, WebDocHit{
					Source:      source,
					Query:       query,
					StartOffset: hit.StartOffset,
					EndOffset:   hit.EndOffset,
					Line:        hit.Line,
					Column:      hit.Column,
					Excerpt:     cleanDocExcerpt(hit.Excerpt),
				})
			}
		}
	}

	sourceOrder := map[string]int{}
	for index, source := range sources {
		sourceOrder[source.Name] = index
	}
	sort.Slice(result.Hits, func(i, j int) bool {
		leftOrder := sourceOrder[result.Hits[i].Source.Name]
		rightOrder := sourceOrder[result.Hits[j].Source.Name]
		if leftOrder == rightOrder {
			return result.Hits[i].StartOffset < result.Hits[j].StartOffset
		}
		return leftOrder < rightOrder
	})
	if cfg.MaxHits > 0 && len(result.Hits) > cfg.MaxHits {
		result.Hits = result.Hits[:cfg.MaxHits]
	}
	result.Found = len(result.Hits) > 0
	return result, nil
}

func BuildDocSearchQueries(question string) []string {
	normalized := strings.TrimSpace(question)
	if normalized == "" {
		return nil
	}
	parts := strings.Fields(webDocNonWordQuery.ReplaceAllString(normalized, " "))
	out := make([]string, 0, 4)
	if len(parts) > 0 {
		out = append(out, strings.Join(parts, " "))
	}
	for _, part := range parts {
		if len(part) >= 4 {
			out = append(out, part)
		}
		if len(out) >= 4 {
			break
		}
	}
	return dedupeStrings(out)
}

func HTMLToSearchableText(raw string) string {
	preserved := extractMetaDescriptions(raw)
	clean := scriptBlockRe.ReplaceAllString(raw, " ")
	clean = styleBlockRe.ReplaceAllString(clean, " ")
	clean = strings.ReplaceAll(clean, "</p>", "</p>\n")
	clean = strings.ReplaceAll(clean, "</h1>", "</h1>\n")
	clean = strings.ReplaceAll(clean, "</h2>", "</h2>\n")
	clean = strings.ReplaceAll(clean, "</h3>", "</h3>\n")
	clean = strings.ReplaceAll(clean, "</li>", "</li>\n")
	clean = strings.ReplaceAll(clean, "</tr>", "</tr>\n")
	clean = htmlTagRe.ReplaceAllString(clean, " ")
	clean = html.UnescapeString(clean)
	if preserved != "" {
		clean = preserved + "\n" + clean
	}
	clean = webDocSentenceEnd.ReplaceAllString(clean, "$1\n")
	clean = whitespaceRe.ReplaceAllString(clean, " ")
	clean = blankLineRe.ReplaceAllString(clean, "\n\n")
	return strings.TrimSpace(clean)
}

func extractMetaDescriptions(raw string) string {
	matches := metaContentRe.FindAllStringSubmatch(raw, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		values = append(values, html.UnescapeString(strings.TrimSpace(match[1])))
	}
	return strings.Join(dedupeStrings(values), "\n")
}

func normalizeWebDocResearchConfig(cfg WebDocResearchConfig) WebDocResearchConfig {
	if cfg.FetchTimeout <= 0 {
		cfg.FetchTimeout = defaultWebDocFetchTimeout
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 3 * 1024 * 1024
	}
	if cfg.MaxHits <= 0 {
		cfg.MaxHits = 12
	}
	cfg.ChunkConfig = normalizeDocumentSearchConfig(cfg.ChunkConfig)
	return cfg
}

func fetchWebDoc(ctx context.Context, url string, cfg WebDocResearchConfig) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", fmt.Errorf("url cannot be empty")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, cfg.FetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "odn-doc-research/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, cfg.MaxBytes)
	blob, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}

func cleanDocExcerpt(raw string) string {
	clean := whitespaceRe.ReplaceAllString(strings.TrimSpace(raw), " ")
	if len(clean) <= 900 {
		return clean
	}
	return clean[:900] + "..."
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
