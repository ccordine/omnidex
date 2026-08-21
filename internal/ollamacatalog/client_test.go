package ollamacatalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchParsesOfficialModelCardsAndPagination(t *testing.T) {
	client := New("https://ollama.example", 0)
	client.httpClient.Transport = catalogRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/search" || request.URL.Query().Get("q") != "dolphin" || request.URL.Query().Get("page") != "1" {
			t.Fatalf("url=%s", request.URL)
		}
		return catalogHTMLResponse(http.StatusOK, `<!doctype html><html><body><ul>
			<li><a href="/library/dolphin3"><h2><span>dolphin3</span></h2><p>Dolphin &amp; friends.</p></a></li>
			<li><a href="/someone/voice-model"><h2><span>someone/voice-model</span></h2><p>Character voice.</p></a></li>
			<li hx-get="/search?page=2&amp;q=dolphin"></li>
		</ul></body></html>`), nil
	})

	page, err := client.Search(context.Background(), "dolphin", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Models) != 2 || !page.HasMore || page.Page != 1 || page.Query != "dolphin" {
		t.Fatalf("page=%+v", page)
	}
	if page.Models[0].Name != "dolphin3" || page.Models[0].Description != "Dolphin & friends." ||
		page.Models[0].URL != "https://ollama.example/library/dolphin3" {
		t.Fatalf("first=%+v", page.Models[0])
	}
	if page.Models[1].Name != "someone/voice-model" {
		t.Fatalf("second=%+v", page.Models[1])
	}
}

func TestSearchRejectsInvalidOrAmbiguousCatalogAuthority(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "name differs", body: `<ul><li><a href="/library/alpha"><h2>beta</h2><p>x</p></a></li></ul>`},
		{name: "duplicate name", body: `<ul><li><a href="/library/alpha"><h2>alpha</h2><p>x</p></a></li><li><a href="/library/alpha"><h2>alpha</h2><p>y</p></a></li></ul>`},
		{name: "external href", body: `<ul><li><a href="https://evil.invalid/model"><h2>model</h2><p>x</p></a></li></ul>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := New("https://ollama.example", 0)
			client.httpClient.Transport = catalogRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return catalogHTMLResponse(http.StatusOK, test.body), nil
			})
			if _, err := client.Search(context.Background(), "alpha", 1); err == nil {
				t.Fatal("expected invalid catalog to fail")
			}
		})
	}
	client := New("https://ollama.example", 0)
	if _, err := client.Search(context.Background(), "", 1); err == nil {
		t.Fatal("expected blank query to fail")
	}
	if _, err := client.Search(context.Background(), "valid", 0); err == nil {
		t.Fatal("expected zero page to fail")
	}
}

type catalogRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn catalogRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func catalogHTMLResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
