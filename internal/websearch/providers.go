package websearch

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

var (
	braveResultRE  = regexp.MustCompile(`(?is)<a\s+href=["'](https?://[^"']+)["'][^>]*class=["'][^"']*\bl1\b[^"']*["'][^>]*>(.*?)</a>`)
	googleResultRE = regexp.MustCompile(`(?is)<a[^>]+href=["'](/url\?q=[^"']+)["'][^>]*>(.*?)</a>`)
	duckResultRE   = regexp.MustCompile(`(?is)<a[^>]+class=["'][^"']*result__a[^"']*["'][^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	yahooResultRE  = regexp.MustCompile(`(?is)<a[^>]+href=["'](https?://[^"']+)["'][^>]*>(.*?)</a>`)
)

type providerDefinition struct {
	id        ProviderID
	searchURL func(string) string
	resultRE  *regexp.Regexp
}

func providerDefinitionFor(id ProviderID) (providerDefinition, bool) {
	definitions := map[ProviderID]providerDefinition{
		ProviderBrave: {
			id: ProviderBrave,
			searchURL: func(query string) string {
				return "https://search.brave.com/search?q=" + url.QueryEscape(query) + "&source=web"
			},
			resultRE: braveResultRE,
		},
		ProviderDuckDuckGo: {
			id: ProviderDuckDuckGo,
			searchURL: func(query string) string {
				return "https://duckduckgo.com/html/?q=" + url.QueryEscape(query)
			},
			resultRE: duckResultRE,
		},
		ProviderGoogle: {
			id: ProviderGoogle,
			searchURL: func(query string) string {
				return "https://www.google.com/search?q=" + url.QueryEscape(query)
			},
			resultRE: googleResultRE,
		},
		ProviderReddit: {
			id: ProviderReddit,
			searchURL: func(query string) string {
				return "https://www.google.com/search?q=" + url.QueryEscape("site:reddit.com "+query)
			},
			resultRE: googleResultRE,
		},
		ProviderYahoo: {
			id: ProviderYahoo,
			searchURL: func(query string) string {
				return "https://search.yahoo.com/search?p=" + url.QueryEscape(query)
			},
			resultRE: yahooResultRE,
		},
	}
	definition, ok := definitions[id]
	return definition, ok
}

type parsedCandidate struct {
	url     string
	title   string
	snippet string
}

func parseProviderCandidates(
	definition providerDefinition,
	searchURL, body string,
	limit int,
) ([]parsedCandidate, error) {
	matches := definition.resultRE.FindAllStringSubmatch(body, limit*4)
	results := make([]parsedCandidate, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		if len(match[1]) > maxURLBytes || len(match[2]) > maxCandidateTextBytes {
			return nil, fmt.Errorf("%w: provider result fields exceed pre-normalization bounds", ErrBoundExceeded)
		}
		rawURL := unwrapProviderURL(definition.id, searchURL, html.UnescapeString(strings.TrimSpace(match[1])))
		canonical, err := CanonicalizeURL(rawURL)
		if err != nil || isProviderOwnedURL(definition.id, canonical) {
			continue
		}
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		title := normalizeHTMLText(match[2])
		results = append(results, parsedCandidate{url: canonical, title: title, snippet: title})
		if len(results) == limit {
			break
		}
	}
	return results, nil
}

func unwrapProviderURL(provider ProviderID, searchURL, rawURL string) string {
	if strings.HasPrefix(rawURL, "/") {
		base, err := url.Parse(searchURL)
		if err != nil {
			return ""
		}
		reference, err := url.Parse(rawURL)
		if err != nil {
			return ""
		}
		rawURL = base.ResolveReference(reference).String()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch provider {
	case ProviderGoogle, ProviderReddit:
		if target := parsed.Query().Get("q"); target != "" {
			return target
		}
	case ProviderDuckDuckGo:
		if target := parsed.Query().Get("uddg"); target != "" {
			return target
		}
	}
	return rawURL
}

func isProviderOwnedURL(provider ProviderID, rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	switch provider {
	case ProviderBrave:
		return host == "search.brave.com" || strings.HasSuffix(host, ".search.brave.com")
	case ProviderGoogle, ProviderReddit:
		return strings.Contains(host, "google.")
	case ProviderDuckDuckGo:
		return host == "duckduckgo.com" || strings.HasSuffix(host, ".duckduckgo.com")
	case ProviderYahoo:
		return host == "yahoo.com" || strings.HasSuffix(host, ".yahoo.com")
	default:
		panic(fmt.Sprintf("unsupported provider %q reached parser", provider))
	}
}
