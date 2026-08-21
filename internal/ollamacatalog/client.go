package ollamacatalog

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	OfficialBaseURL         = "https://ollama.com"
	MaxCatalogResponseBytes = 4 * 1024 * 1024
	MaxCatalogResults       = 100
)

type Model struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type Page struct {
	Query   string  `json:"query"`
	Page    int     `json:"page"`
	Models  []Model `json:"models"`
	HasMore bool    `json:"has_more"`
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	parsed, _ := url.Parse(baseURL)
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.DisableCompression = true
	return &Client{baseURL: parsed, httpClient: &http.Client{Timeout: timeout, Transport: transport}}
}

func NewOfficial(timeout time.Duration) *Client { return New(OfficialBaseURL, timeout) }

func (client *Client) Search(ctx context.Context, query string, pageNumber int) (Page, error) {
	if ctx == nil || client == nil || client.baseURL == nil || client.httpClient == nil ||
		(client.baseURL.Scheme != "https" && client.baseURL.Scheme != "http") || client.baseURL.Host == "" {
		return Page{}, fmt.Errorf("Ollama catalog client is not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 128 || !utf8.ValidString(query) || strings.ContainsRune(query, '\x00') {
		return Page{}, fmt.Errorf("Ollama catalog query must be 1..128 bytes of canonical UTF-8")
	}
	if pageNumber < 1 || pageNumber > 100 {
		return Page{}, fmt.Errorf("Ollama catalog page must be between 1 and 100")
	}
	endpoint := *client.baseURL
	endpoint.Path = "/search"
	values := endpoint.Query()
	values.Set("q", query)
	values.Set("page", strconv.Itoa(pageNumber))
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Page{}, err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "Omnidex-Ollama-Catalog/1")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return Page{}, fmt.Errorf("search Ollama catalog: %w", err)
	}
	defer response.Body.Close()
	limited := &io.LimitedReader{R: response.Body, N: MaxCatalogResponseBytes + 1}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(limited)
		if readErr != nil {
			return Page{}, readErr
		}
		return Page{}, fmt.Errorf("Ollama catalog search failed: status=%d body=%s", response.StatusCode, body)
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])); mediaType != "text/html" {
		return Page{}, fmt.Errorf("Ollama catalog returned content type %q", response.Header.Get("Content-Type"))
	}
	document, err := html.Parse(limited)
	if err != nil {
		return Page{}, fmt.Errorf("parse Ollama catalog HTML: %w", err)
	}
	if limited.N == 0 {
		return Page{}, fmt.Errorf("Ollama catalog response exceeds %d bytes", MaxCatalogResponseBytes)
	}
	return parseCatalogDocument(client.baseURL, document, query, pageNumber)
}

func parseCatalogDocument(base *url.URL, document *html.Node, query string, pageNumber int) (Page, error) {
	result := Page{Query: query, Page: pageNumber, Models: make([]Model, 0)}
	seen := make(map[string]struct{})
	var visit func(*html.Node) error
	visit = func(node *html.Node) error {
		if node.Type == html.ElementNode && node.Data == "li" {
			if next, exists := attribute(node, "hx-get"); exists && strings.HasPrefix(next, "/search") {
				hasMore, err := isNextCatalogPage(next, query, pageNumber)
				if err != nil {
					return err
				}
				result.HasMore = result.HasMore || hasMore
			}
			anchor := firstElement(node, "a")
			heading := firstElement(node, "h2")
			if heading != nil {
				if anchor == nil {
					return fmt.Errorf("Ollama catalog model card is missing its link")
				}
				href, exists := attribute(anchor, "href")
				if !exists {
					return fmt.Errorf("Ollama catalog model card is missing its href")
				}
				name, err := modelNameFromPath(href)
				if err != nil {
					return err
				}
				displayed := canonicalText(textContent(heading))
				if displayed != name {
					return fmt.Errorf("Ollama catalog model %q differs from href identity %q", displayed, name)
				}
				if _, duplicate := seen[name]; duplicate {
					return fmt.Errorf("Ollama catalog duplicates model %q", name)
				}
				seen[name] = struct{}{}
				description := ""
				if paragraph := firstElement(anchor, "p"); paragraph != nil {
					description = canonicalText(textContent(paragraph))
				}
				if len(description) > 2048 {
					return fmt.Errorf("Ollama catalog description for %q exceeds 2048 bytes", name)
				}
				modelURL := *base
				modelURL.Path = path.Clean(href)
				modelURL.RawQuery = ""
				modelURL.Fragment = ""
				result.Models = append(result.Models, Model{Name: name, Description: description, URL: modelURL.String()})
				if len(result.Models) > MaxCatalogResults {
					return fmt.Errorf("Ollama catalog page exceeds %d models", MaxCatalogResults)
				}
				return nil
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(document); err != nil {
		return Page{}, err
	}
	return result, nil
}

func modelNameFromPath(href string) (string, error) {
	parsed, err := url.Parse(href)
	if err != nil || parsed.IsAbs() || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/") {
		return "", fmt.Errorf("Ollama catalog model href %q is invalid", href)
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) == 2 && parts[0] == "library" {
		parts = parts[1:]
	} else if len(parts) != 2 {
		return "", fmt.Errorf("Ollama catalog model href %q has unsupported shape", href)
	}
	for _, part := range parts {
		if part == "" || part != strings.TrimSpace(part) || len(part) > 128 {
			return "", fmt.Errorf("Ollama catalog model href %q is invalid", href)
		}
		for _, character := range part {
			if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
				continue
			}
			return "", fmt.Errorf("Ollama catalog model href %q contains unsupported text", href)
		}
	}
	return strings.Join(parts, "/"), nil
}

func isNextCatalogPage(href, query string, current int) (bool, error) {
	parsed, err := url.Parse(href)
	if err != nil || parsed.IsAbs() || parsed.Path != "/search" {
		return false, fmt.Errorf("Ollama catalog pagination href %q is invalid", href)
	}
	next, err := strconv.Atoi(parsed.Query().Get("page"))
	if err != nil || parsed.Query().Get("q") != query {
		return false, fmt.Errorf("Ollama catalog pagination differs from its search authority")
	}
	return next == current+1, nil
}

func firstElement(node *html.Node, name string) *html.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			return child
		}
		if nested := firstElement(child, name); nested != nil {
			return nested
		}
	}
	return nil
}

func attribute(node *html.Node, name string) (string, bool) {
	for _, item := range node.Attr {
		if item.Key == name {
			return item.Val, true
		}
	}
	return "", false
}

func textContent(node *html.Node) string {
	var value strings.Builder
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			value.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return value.String()
}

func canonicalText(value string) string { return strings.Join(strings.Fields(value), " ") }
