package main

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

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
