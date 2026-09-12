package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type observedResponseTransport func(*http.Request) (*http.Response, error)

func (transport observedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type observedPublicResolver struct{}

func (observedPublicResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func TestDiscoveryAndFetchRetainActualValuesWithReportLocalReferences(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ title, text, address string }{
		{"Observatory", "The recorded temperature is twelve degrees.", "https://observations.example/reading"},
		{"Archive", "The reading room opens on Wednesday.", "https://archive.example/opening"},
	} {
		t.Run(fixture.title, func(t *testing.T) {
			calls := 0
			bodyText := fixture.text
			service, err := New(Config{
				Providers: []ProviderID{ProviderBrave, ProviderDuckDuckGo}, Timeout: time.Second,
				PerDocumentBytes: 4096, TotalDocumentBytes: 4096,
				MaxCandidatesPerProvider: 1, MaxCandidates: 2, MaxDocuments: 1, MaxResponseBytes: 8192,
				Resolver: observedPublicResolver{}, HTTPClient: &http.Client{Transport: observedResponseTransport(
					func(request *http.Request) (*http.Response, error) {
						calls++
						if request.Method != http.MethodGet {
							t.Fatalf("unexpected method %s", request.Method)
						}
						body := `<title>` + fixture.title + `</title><p>` + bodyText + `</p>`
						switch request.URL.Hostname() {
						case "search.brave.com":
							body = `<a href="` + fixture.address + `" class="l1">` + fixture.title + `</a>`
						case "duckduckgo.com":
							body = `<a class="result__a" href="` + fixture.address + `">` + fixture.title + `</a>`
						default:
							if request.URL.String() != fixture.address {
								t.Fatalf("unexpected fetched URL %s", request.URL)
							}
						}
						return &http.Response{StatusCode: 200, Header: make(http.Header),
							Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
					}),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			discovery, err := service.Discover(ctx, QueryRequest{Query: fixture.title})
			if err != nil || len(discovery.Candidates) != 1 {
				t.Fatalf("discovery: %#v / %v", discovery, err)
			}
			candidate := discovery.Candidates[0]
			if candidate.ID != "candidate_1" || candidate.URL != fixture.address || len(candidate.Sources) != 2 {
				t.Fatalf("URL-based deduplication lost actual discovery sources: %#v", candidate)
			}
			request := FetchRequest{Candidates: discovery.Candidates, CandidateIDs: []CandidateID{candidate.ID}}
			first, err := service.Fetch(ctx, request)
			if err != nil || len(first.Documents) != 1 {
				t.Fatalf("first fetch: %#v / %v", first, err)
			}
			document := first.Documents[0]
			if document.ID != "document_1" || document.CandidateID != candidate.ID || document.URL != fixture.address ||
				!strings.Contains(document.Content, fixture.text) || document.ObservedAt.IsZero() {
				t.Fatalf("actual fetch values differ: %#v", document)
			}
			if err := ValidateDocument(document); err != nil {
				t.Fatal(err)
			}
			bodyText = "A later source observation contains different text."
			second, err := service.Fetch(ctx, request)
			if err != nil || second.Documents[0].ID != document.ID || second.Documents[0].Content == document.Content {
				t.Fatalf("a report-local reference became content-addressed: %#v / %v", second, err)
			}
			encoded, err := json.Marshal(document)
			if err != nil || strings.Contains(strings.ToLower(string(encoded)), "sha256") {
				t.Fatalf("fetched evidence retained a content hash: %s / %v", encoded, err)
			}
			request.CandidateIDs[0] = "candidate_2"
			if _, err := service.Fetch(ctx, request); err == nil || calls != 4 {
				t.Fatalf("unknown selection executed a fetch: calls=%d, error=%v", calls, err)
			}
			if _, err := service.get(ctx, "http://127.0.0.1/private"); err == nil || calls != 4 {
				t.Fatalf("private destination reached transport: calls=%d, error=%v", calls, err)
			}
			for _, mutate := range []func(*Document){
				func(d *Document) { d.ID = "" },
				func(d *Document) { d.CandidateID = CandidateID("candidate_" + strings.Repeat("a", 64)) },
				func(d *Document) { d.Content = "invalid\x00text" },
				func(d *Document) { d.ObservedAt = time.Time{} },
			} {
				invalid := document
				mutate(&invalid)
				if err := ValidateDocument(invalid); err == nil {
					t.Fatalf("invalid source value accepted: %#v", invalid)
				}
			}
		})
	}
}
