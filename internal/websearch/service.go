package websearch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultTimeout        = 15 * time.Second
	defaultPerSourceLimit = 3000
	defaultTotalLimit     = 6000
	defaultMaxCandidates  = 4
	maxBodyBytes          = 2 << 20
)

var (
	invalidQueryChars = regexp.MustCompile(`[^a-zA-Z0-9\+]+`)
	multiPlus         = regexp.MustCompile(`[\+]+`)
	htmlCommentRE     = regexp.MustCompile(`(?is)<!--.*?-->`)
	headRE            = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	scriptRE          = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRE           = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRE        = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	metaRE            = regexp.MustCompile(`(?is)<meta[^>]*>`)
	tagRE             = regexp.MustCompile(`(?is)<[^>]+>`)
	whitespaceRE      = regexp.MustCompile(`\s+`)
	anchorRE          = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"'#]+)["'][^>]*>(.*?)</a>`)
	titleRE           = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaDescriptionRE = regexp.MustCompile(`(?is)<meta[^>]+(?:name=["']description["']|property=["']og:description["'])[^>]+content=["']([^"']+)["'][^>]*>`)
	googleResultRE    = regexp.MustCompile(`(?is)<a[^>]+href=["'](/url\?q=[^"']+)["'][^>]*>(.*?)</a>`)
	duckResultRE      = regexp.MustCompile(`(?is)<a[^>]+class=["'][^"']*result__a[^"']*["'][^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	yahooResultRE     = regexp.MustCompile(`(?is)<a[^>]+href=["'](https?://[^"']+)["'][^>]*>(.*?)</a>`)
)

type Provider struct {
	Name        string
	URLTemplate string
}

type SearchCandidate struct {
	Provider  string `json:"provider"`
	SearchURL string `json:"search_url,omitempty"`
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
}

type Result struct {
	Provider    string    `json:"provider"`
	SearchURL   string    `json:"search_url,omitempty"`
	URL         string    `json:"url"`
	Title       string    `json:"title,omitempty"`
	Snippet     string    `json:"snippet,omitempty"`
	Content     string    `json:"content"`
	RetrievedAt time.Time `json:"retrieved_at,omitempty"`
}

type Service struct {
	providers       []Provider
	perSourceBudget int
	totalBudget     int
	maxCandidates   int
	httpClient      *http.Client
}

func New(providerNames []string, timeout time.Duration, perSourceBudget, totalBudget int) *Service {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if perSourceBudget <= 0 {
		perSourceBudget = defaultPerSourceLimit
	}
	if totalBudget <= 0 {
		totalBudget = defaultTotalLimit
	}
	return &Service{
		providers:       resolveProviders(providerNames),
		perSourceBudget: perSourceBudget,
		totalBudget:     totalBudget,
		maxCandidates:   defaultMaxCandidates,
		httpClient:      &http.Client{Timeout: timeout},
	}
}

func (s *Service) Search(ctx context.Context, query string) (string, error) {
	results, err := s.SearchAll(ctx, query)
	if err != nil {
		return "", err
	}
	return BuildContext(results, s.totalBudget), nil
}

func (s *Service) SearchAll(ctx context.Context, query string) ([]Result, error) {
	query = NormalizeQuery(query)
	if query == "" {
		return nil, errors.New("search query is empty after normalization")
	}
	if len(s.providers) == 0 {
		return nil, errors.New("no web search providers configured")
	}

	seen := map[string]struct{}{}
	results := make([]Result, 0, len(s.providers)*s.maxCandidates)
	var lastErr error
	for _, provider := range s.providers {
		searchURL := fmt.Sprintf(provider.URLTemplate, query)
		body, err := s.fetchBody(ctx, searchURL)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", provider.Name, err)
			continue
		}
		candidates := extractCandidates(provider, searchURL, body, s.maxCandidates)
		fetched := 0
		for _, candidate := range candidates {
			if _, ok := seen[candidate.URL]; ok {
				continue
			}
			doc, err := s.fetchDocument(ctx, candidate.URL)
			if err != nil || strings.TrimSpace(doc.Content) == "" {
				continue
			}
			seen[candidate.URL] = struct{}{}
			title := strings.TrimSpace(candidate.Title)
			if title == "" {
				title = doc.Title
			}
			snippet := strings.TrimSpace(candidate.Snippet)
			if snippet == "" {
				snippet = doc.Snippet
			}
			results = append(results, Result{
				Provider:    provider.Name,
				SearchURL:   searchURL,
				URL:         candidate.URL,
				Title:       title,
				Snippet:     snippet,
				Content:     truncate(doc.Content, s.perSourceBudget),
				RetrievedAt: time.Now().UTC(),
			})
			fetched++
			if fetched >= s.maxCandidates {
				break
			}
		}
		if fetched > 0 {
			continue
		}
		fallback := truncate(extractText(body), s.perSourceBudget)
		if strings.TrimSpace(fallback) == "" {
			continue
		}
		results = append(results, Result{
			Provider:    provider.Name,
			SearchURL:   searchURL,
			URL:         searchURL,
			Title:       provider.Name + " search results",
			Content:     fallback,
			RetrievedAt: time.Now().UTC(),
		})
	}
	if len(results) == 0 {
		if lastErr == nil {
			lastErr = errors.New("all providers returned empty results")
		}
		return nil, lastErr
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Provider == results[j].Provider {
			return results[i].URL < results[j].URL
		}
		return results[i].Provider < results[j].Provider
	})
	return results, nil
}

func NormalizeQuery(value string) string {
	query := strings.TrimSpace(value)
	if query == "" {
		return ""
	}
	query = strings.ReplaceAll(query, ",", "+")
	query = strings.ReplaceAll(query, " ", "+")
	query = invalidQueryChars.ReplaceAllString(query, "")
	query = multiPlus.ReplaceAllString(query, "+")
	query = strings.Trim(query, "+")
	return query
}

func BuildContext(results []Result, budget int) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	for _, result := range results {
		title := strings.TrimSpace(result.Title)
		if title == "" {
			title = result.URL
		}
		segment := fmt.Sprintf(
			"Provider: %s\nTitle: %s\nURL: %s\nSearch URL: %s\nSnippet: %s\nContent:\n%s\n\n",
			result.Provider,
			title,
			result.URL,
			strings.TrimSpace(result.SearchURL),
			strings.TrimSpace(result.Snippet),
			result.Content,
		)
		if budget > 0 && b.Len()+len(segment) > budget {
			remaining := budget - b.Len()
			if remaining <= 0 {
				break
			}
			segment = segment[:remaining]
		}
		b.WriteString(segment)
		if budget > 0 && b.Len() >= budget {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

type fetchedDocument struct {
	Title   string
	Snippet string
	Content string
}

func (s *Service) fetchDocument(ctx context.Context, url string) (fetchedDocument, error) {
	body, err := s.fetchBody(ctx, url)
	if err != nil {
		return fetchedDocument{}, err
	}
	return fetchedDocument{
		Title:   extractTitle(body),
		Snippet: extractDescription(body),
		Content: extractText(body),
	}, nil
}

func (s *Service) fetchBody(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; omnidex/1.0)")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, truncate(strings.TrimSpace(string(body)), 240))
	}
	return string(body), nil
}

func extractCandidates(provider Provider, searchURL, body string, limit int) []SearchCandidate {
	if limit <= 0 {
		limit = defaultMaxCandidates
	}
	searchHost := ""
	if parsed, err := neturl.Parse(searchURL); err == nil {
		searchHost = strings.ToLower(parsed.Host)
	}
	seen := map[string]struct{}{}
	out := make([]SearchCandidate, 0, limit)
	for _, candidate := range providerSpecificCandidates(provider.Name, searchURL, body, searchHost) {
		cleaned := cleanCandidateURL(candidate.URL, searchHost)
		if cleaned == "" || isIgnoredCandidateURL(cleaned, searchHost) {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		candidate.URL = cleaned
		out = append(out, candidate)
		if len(out) >= limit {
			return out
		}
	}
	matches := anchorRE.FindAllStringSubmatch(body, limit*12)
	for _, match := range matches {
		href := strings.TrimSpace(html.UnescapeString(match[1]))
		title := strings.TrimSpace(extractText(match[2]))
		cleaned := cleanCandidateURL(href, searchHost)
		if cleaned == "" || isIgnoredCandidateURL(cleaned, searchHost) {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, SearchCandidate{Provider: provider.Name, SearchURL: searchURL, URL: cleaned, Title: title, Snippet: title})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func providerSpecificCandidates(providerName, searchURL, body, searchHost string) []SearchCandidate {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	var re *regexp.Regexp
	switch providerName {
	case "google", "reddit":
		re = googleResultRE
	case "duckduckgo":
		re = duckResultRE
	case "yahoo":
		re = yahooResultRE
	default:
		return nil
	}
	matches := re.FindAllStringSubmatch(body, 24)
	out := make([]SearchCandidate, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(match[1]))
		title := strings.TrimSpace(extractText(match[2]))
		cleaned := cleanCandidateURL(href, searchHost)
		if cleaned == "" || isIgnoredCandidateURL(cleaned, searchHost) {
			continue
		}
		out = append(out, SearchCandidate{Provider: providerName, SearchURL: searchURL, URL: cleaned, Title: title, Snippet: title})
	}
	return out
}

func cleanCandidateURL(rawHref, searchHost string) string {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return ""
	}
	if strings.HasPrefix(rawHref, "//") {
		rawHref = "https:" + rawHref
	}
	if strings.HasPrefix(rawHref, "/url?") {
		rawHref = "https://" + searchHost + rawHref
	}
	parsed, err := neturl.Parse(rawHref)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(parsed.Host), "google.") {
		for _, key := range []string{"q", "url"} {
			if q := parsed.Query().Get(key); q != "" {
				resolved, err := neturl.Parse(q)
				if err == nil && (resolved.Scheme == "http" || resolved.Scheme == "https") {
					return resolved.String()
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(parsed.Host), "duckduckgo.") {
		if uddg := parsed.Query().Get("uddg"); uddg != "" {
			resolved, err := neturl.Parse(uddg)
			if err == nil && (resolved.Scheme == "http" || resolved.Scheme == "https") {
				return resolved.String()
			}
		}
	}
	return parsed.String()
}

func isIgnoredCandidateURL(rawURL, searchHost string) bool {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Host)
	if host == "" {
		return true
	}
	if searchHost != "" && strings.Contains(host, searchHost) {
		return true
	}
	if strings.HasPrefix(strings.ToLower(parsed.Path), "/search") || strings.HasPrefix(strings.ToLower(parsed.Path), "/preferences") || strings.HasPrefix(strings.ToLower(parsed.Path), "/settings") {
		return true
	}
	return false
}

func extractTitle(body string) string {
	match := titleRE.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return truncate(extractText(match[1]), 240)
}

func extractDescription(body string) string {
	match := metaDescriptionRE.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return truncate(html.UnescapeString(strings.TrimSpace(match[1])), 300)
}

func extractText(body string) string {
	clean := htmlCommentRE.ReplaceAllString(body, " ")
	clean = headRE.ReplaceAllString(clean, " ")
	clean = scriptRE.ReplaceAllString(clean, " ")
	clean = styleRE.ReplaceAllString(clean, " ")
	clean = noscriptRE.ReplaceAllString(clean, " ")
	clean = metaRE.ReplaceAllString(clean, " ")
	clean = tagRE.ReplaceAllString(clean, " ")
	clean = html.UnescapeString(clean)
	clean = whitespaceRE.ReplaceAllString(clean, " ")
	return strings.TrimSpace(clean)
}

func resolveProviders(providerNames []string) []Provider {
	known := map[string]Provider{
		"google":     {Name: "google", URLTemplate: "https://www.google.com/search?q=%s"},
		"yahoo":      {Name: "yahoo", URLTemplate: "https://search.yahoo.com/search?p=%s"},
		"reddit":     {Name: "reddit", URLTemplate: "https://www.google.com/search?q=site%3Areddit.com+%s"},
		"duckduckgo": {Name: "duckduckgo", URLTemplate: "https://duckduckgo.com/html/?q=%s"},
	}
	if len(providerNames) == 0 {
		return []Provider{known["duckduckgo"], known["yahoo"], known["google"], known["reddit"]}
	}
	seen := map[string]struct{}{}
	out := make([]Provider, 0, len(providerNames))
	for _, raw := range providerNames {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if provider, ok := known[name]; ok {
			out = append(out, provider)
		}
	}
	if len(out) == 0 {
		return []Provider{known["duckduckgo"], known["yahoo"], known["google"], known["reddit"]}
	}
	return out
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	if max < 20 {
		return value[:max]
	}
	return value[:max-15] + "\n...[truncated]"
}
