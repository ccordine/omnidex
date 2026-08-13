package websearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFetchRetrievesOnlyExplicitCandidatesWithoutFollowingLinks(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://docs.example/primary": {
			status: http.StatusOK,
			body: `<html><head><title>Primary document</title><meta name="description" content="Bounded description"></head>
			<body>Primary evidence.<a href="https://docs.example/arbitrary">Do not follow</a></body></html>`,
		},
	}}
	config := validConfig(transport, ProviderDuckDuckGo)
	config.MaxCandidates = 4
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testCandidate(t, "https://docs.example/primary")
	report, err := service.Fetch(context.Background(), FetchRequest{
		Candidates:   []Candidate{candidate},
		CandidateIDs: []CandidateID{candidate.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(transport.requests); got != 1 {
		t.Fatalf("HTTP requests=%d want 1; arbitrary page link was followed", got)
	}
	if len(report.Documents) != 1 || len(report.Diagnostics) != 1 {
		t.Fatalf("report=%#v", report)
	}
	document := report.Documents[0]
	if document.URL != candidate.URL || !strings.Contains(document.Content, "Primary evidence") {
		t.Fatalf("document=%#v", document)
	}
	digest := sha256.Sum256([]byte(document.Content))
	if document.ContentSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("content SHA=%q", document.ContentSHA256)
	}
	if document.ID == "" || !strings.HasPrefix(string(document.ID), "document_") {
		t.Fatalf("document ID=%q", document.ID)
	}
	if document.ObservedAt.IsZero() || document.ObservedAt.Location() != time.UTC {
		t.Fatalf("document observation authority=%v", document.ObservedAt)
	}
}

func TestFetchRejectsUnknownDuplicateAndTamperedCandidateIDs(t *testing.T) {
	service, err := New(validConfig(&fixtureTransport{}, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	candidate := testCandidate(t, "https://docs.example/known")
	tests := []struct {
		name    string
		request FetchRequest
		want    string
	}{
		{name: "unknown", request: FetchRequest{Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{"candidate_unknown"}}, want: "unknown candidate"},
		{name: "duplicate", request: FetchRequest{Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{candidate.ID, candidate.ID}}, want: "duplicate candidate"},
		{name: "tampered", request: FetchRequest{Candidates: []Candidate{{ID: "candidate_tampered", URL: candidate.URL, Sources: candidate.Sources}}, CandidateIDs: []CandidateID{"candidate_tampered"}}, want: "does not match canonical URL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Fetch(context.Background(), test.request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Fetch error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestFetchFailsWhenNoUsableDocumentExists(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://docs.example/empty": {status: http.StatusOK, body: `<html><head></head><body><script>ignored()</script></body></html>`},
	}}
	service, err := New(validConfig(transport, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	candidate := testCandidate(t, "https://docs.example/empty")
	report, err := service.Fetch(context.Background(), FetchRequest{Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{candidate.ID}})
	if err == nil || !strings.Contains(err.Error(), "no usable documents") {
		t.Fatalf("Fetch error=%v", err)
	}
	if len(report.Documents) != 0 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Outcome != FetchEmpty {
		t.Fatalf("report=%#v", report)
	}
}

func TestFetchRejectsPublicDocumentRedirectBeforeRequestingTarget(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://source.example/document": {status: http.StatusFound, body: "redirect"},
	}}
	redirect := policyRedirectTransport{fixtureTransport: transport, location: "https://target.example/final"}
	config := validConfig(redirect, ProviderDuckDuckGo)
	config.Resolver = fixtureResolver{addresses: map[string][]net.IPAddr{
		"source.example": {{IP: net.ParseIP("93.184.216.34")}},
		"target.example": {{IP: net.ParseIP("93.184.216.35")}},
	}}
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testCandidate(t, "https://source.example/document")
	report, err := service.Fetch(context.Background(), FetchRequest{
		Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{candidate.ID},
	})
	if !errors.Is(err, ErrDocumentRedirect) {
		t.Fatalf("Fetch error=%v want ErrDocumentRedirect", err)
	}
	if len(report.Documents) != 0 {
		t.Fatalf("redirect was projected as source document: %#v", report.Documents)
	}
	if len(transport.requests) != 1 || transport.requests[0] != candidate.URL {
		t.Fatalf("requests=%v; redirect target must not be requested", transport.requests)
	}
}

func TestFetchCancellationAfterDocumentReadReturnsNoPartialAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	service, err := New(validConfig(cancelAfterReadTransport{
		cancel: cancel, body: "<html><body>Exact evidence.</body></html>",
	}, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	candidate := testCandidate(t, "https://docs.example/canceled")
	report, err := service.Fetch(ctx, FetchRequest{
		Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{candidate.ID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error=%v want cancellation", err)
	}
	if len(report.Documents) != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("canceled fetch leaked partial authority: %#v", report)
	}
}

func TestFetchRejectsInvalidUTF8AndNULBodiesBeforeProjection(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "invalid UTF-8", body: string([]byte{'e', 'v', 'i', 'd', 'e', 'n', 'c', 'e', 0xff})},
		{name: "NUL", body: "evidence\x00hidden"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const rawURL = "https://docs.example/invalid-text"
			transport := &fixtureTransport{responses: map[string]fixtureResponse{
				rawURL: {status: http.StatusOK, body: test.body},
			}}
			service, err := New(validConfig(transport, ProviderDuckDuckGo))
			if err != nil {
				t.Fatal(err)
			}
			candidate := testCandidate(t, rawURL)
			report, err := service.Fetch(context.Background(), FetchRequest{
				Candidates: []Candidate{candidate}, CandidateIDs: []CandidateID{candidate.ID},
			})
			if !errors.Is(err, ErrInvalidFetchedText) {
				t.Fatalf("Fetch error=%v want ErrInvalidFetchedText", err)
			}
			if len(report.Documents) != 0 {
				t.Fatalf("invalid body reached projection: %#v", report.Documents)
			}
		})
	}
}

func TestValidationRejectsInvalidUTF8AndNULProjectedText(t *testing.T) {
	candidate := testCandidate(t, "https://docs.example/text")
	for _, mutate := range []func(*Candidate){
		func(value *Candidate) { value.Title = string([]byte{0xff}) },
		func(value *Candidate) { value.Snippet = "visible\x00hidden" },
	} {
		value := candidate
		mutate(&value)
		if err := ValidateCandidate(value); !errors.Is(err, ErrInvalidFetchedText) {
			t.Fatalf("ValidateCandidate error=%v want ErrInvalidFetchedText", err)
		}
	}

	content := "valid content"
	digest := sha256.Sum256([]byte(content))
	contentSHA := hex.EncodeToString(digest[:])
	document := Document{
		ID: documentID(candidate.URL, contentSHA), CandidateID: candidate.ID, URL: candidate.URL,
		Title: "valid", Snippet: "valid", Content: content, ContentSHA256: contentSHA,
		ObservedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
	for _, mutate := range []func(*Document){
		func(value *Document) { value.Title = string([]byte{0xff}) },
		func(value *Document) { value.Snippet = "visible\x00hidden" },
		func(value *Document) { value.Content = "visible\x00hidden" },
	} {
		value := document
		mutate(&value)
		if err := ValidateDocument(value); !errors.Is(err, ErrInvalidFetchedText) {
			t.Fatalf("ValidateDocument error=%v want ErrInvalidFetchedText", err)
		}
	}
}

func testCandidate(t *testing.T, rawURL string) Candidate {
	t.Helper()
	canonical, err := CanonicalizeURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return Candidate{
		ID: candidateID(canonical), URL: canonical, Title: "Fixture",
		Sources: []CandidateSource{{Provider: ProviderDuckDuckGo, SearchURL: "https://duckduckgo.com/html/?q=fixture", Rank: 1}},
	}
}
