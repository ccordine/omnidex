package websearch

import (
	"fmt"
	"strings"
	"testing"
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
