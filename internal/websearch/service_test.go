package websearch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedditProviderTemplateEscapesPercentEncoding(t *testing.T) {
	providers := resolveProviders([]string{"reddit"})
	if len(providers) != 1 {
		t.Fatalf("providers=%d", len(providers))
	}
	url := fmt.Sprintf(providers[0].URLTemplate, "rust+async")
	if strings.Contains(url, "%!") {
		t.Fatalf("reddit URL template produced fmt artifact: %s", url)
	}
	if !strings.Contains(url, "site%3Areddit.com+rust+async") {
		t.Fatalf("reddit URL template did not preserve site filter: %s", url)
	}
}

func TestLowQualitySearchResultRejectsGoogleFeedback(t *testing.T) {
	if !isLowQualitySearchResult("https://support.google.com/websearch", "feedback", "", "Google Search Help Submit feedback") {
		t.Fatal("expected Google feedback/help page to be rejected")
	}
	if !isLowQualitySearchResult("https://www.google.com/search?q=rust", "google search results", "", "If you're having trouble accessing Google Search, please click here, or send feedback.") {
		t.Fatal("expected Google blocked-search fallback to be rejected")
	}
	if !isLowQualitySearchResult("https://search.yahoo.com/search?p=rust", "yahoo search results", "", "Yahoo has ceased search operations in Thailand.") {
		t.Fatal("expected unavailable Yahoo fallback to be rejected")
	}
	if isLowQualitySearchResult("https://doc.rust-lang.org/book/", "The Rust Programming Language", "", "Ownership Cargo crates") {
		t.Fatal("expected official Rust docs to be usable")
	}
}

func TestSearchAllFollowsUsefulResultLinks(t *testing.T) {
	contentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			fmt.Fprintf(w, `<html><head><title>Primary page</title></head><body>Primary page content.<a href="/details">Deep detail</a></body></html>`)
		case "/details":
			fmt.Fprintf(w, `<html><head><title>Deep detail</title></head><body>Deep detail content found by following a page link.</body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer contentServer.Close()
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><a href="%s/page">Primary result</a></body></html>`, contentServer.URL)
	}))
	defer searchServer.Close()

	service := New(nil, time.Second, 2000, 6000)
	service.providers = []Provider{{Name: "test", URLTemplate: searchServer.URL + "/search?q=%s"}}
	service.maxCandidates = 1
	service.maxFollowLinks = 1
	results, err := service.SearchAll(context.Background(), "deep detail")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range results {
		if strings.Contains(result.Content, "Deep detail content") && strings.Contains(result.Provider, "follow") {
			found = true
		}
	}
	if !found {
		t.Fatalf("followed detail page missing: %#v", results)
	}
}

func TestResolveProvidersDefaultsSkipYahoo(t *testing.T) {
	providers := resolveProviders(nil)
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	got := strings.Join(names, ",")
	if got != "duckduckgo,google,reddit" {
		t.Fatalf("default providers=%s want duckduckgo,google,reddit", got)
	}
}

func TestSearchAllDetailedContinuesAfterProviderFailure(t *testing.T) {
	contentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Good page</title></head><body>Useful provider result.</body></html>`)
	}))
	defer contentServer.Close()
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><a href="%s/page">Good result</a></body></html>`, contentServer.URL)
	}))
	defer searchServer.Close()

	service := New(nil, time.Second, 2000, 6000)
	service.providers = []Provider{
		{Name: "broken", URLTemplate: "http://127.0.0.1:1/search?q=%s"},
		{Name: "good", URLTemplate: searchServer.URL + "/search?q=%s"},
	}

	report, err := service.SearchAllDetailed(context.Background(), "provider fallback")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) == 0 {
		t.Fatal("expected successful provider results")
	}
	if len(report.Diagnostics) != 2 {
		t.Fatalf("diagnostics=%d want 2", len(report.Diagnostics))
	}
	if report.Diagnostics[0].Provider != "broken" || report.Diagnostics[0].Error == "" {
		t.Fatalf("first diagnostic should record broken provider failure: %#v", report.Diagnostics[0])
	}
	if report.Diagnostics[1].Provider != "good" || !report.Diagnostics[1].Succeeded {
		t.Fatalf("second diagnostic should record good provider success: %#v", report.Diagnostics[1])
	}
}
