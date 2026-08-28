package webresearch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestProductionAcquisitionBoundFetchesExactlyTwoOfThreeCandidates(t *testing.T) {
	const searchURL = "https://www.google.com/search?q=bounded+integration"
	transport := &machineFixtureTransport{responses: map[string]string{
		searchURL: `<a href="/url?q=https%3A%2F%2Fone.example%2Fdoc">One</a>
			<a href="/url?q=https%3A%2F%2Ftwo.example%2Fdoc">Two</a>
			<a href="/url?q=https%3A%2F%2Fthree.example%2Fdoc">Three</a>`,
		"https://one.example/doc":   `<title>One</title><body>Exact first evidence.</body>`,
		"https://two.example/doc":   `<title>Two</title><body>Exact second evidence.</body>`,
		"https://three.example/doc": `<title>Three</title><body>Must not be fetched.</body>`,
	}}
	service, err := websearch.New(websearch.Config{
		Providers: []websearch.ProviderID{websearch.ProviderGoogle}, Timeout: time.Second,
		PerDocumentBytes: 1_000, TotalDocumentBytes: 2_000,
		MaxCandidatesPerProvider: 4, MaxCandidates: 4, MaxDocuments: 2,
		MaxResponseBytes: 8_000, HTTPClient: &http.Client{Transport: transport},
		Resolver: machineFixtureResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	relevance := &selectFirstCandidateStation{}
	synthesis := &synthesizeFirstEvidenceStation{}
	machine, err := New(Objective{
		ID: "objective_service_integration", Question: "What is established?",
		InitialQuery: "bounded integration", Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, Config{
		MaxSearchTerms: 3, MaxSearchTermBytes: 120, MaxFetchCandidates: 2,
		MaxProjectionBytes: 2_000, MaxRelevantCandidates: 2, CandidateSummaryBytes: 240,
		MaxSynthesisParagraphs: 4, MaxSynthesisParagraphBytes: 1_000,
	}, service, &recordingTermsStation{}, relevance, synthesis,
		&recordingSynthesisCorrectionStation{}, &recordingClaimEvidenceReviewStation{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.FetchAttempts != 1 || len(result.Fetches) != 1 ||
		len(result.Fetches[0].Documents) != 2 {
		t.Fatalf("result completion/fetch authority=%+v", result)
	}
	if got := strings.Join(transport.requests, "\n"); strings.Contains(got, "three.example") ||
		len(transport.requests) != 3 {
		t.Fatalf("HTTP requests=%v; expected one discovery plus exactly two documents", transport.requests)
	}
}

type machineFixtureTransport struct {
	responses map[string]string
	requests  []string
}

func (transport *machineFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	rawURL := request.URL.String()
	transport.requests = append(transport.requests, rawURL)
	body, ok := transport.responses[rawURL]
	if !ok {
		return nil, fmt.Errorf("unexpected machine integration request %s", rawURL)
	}
	return &http.Response{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)),
		Header: make(http.Header), Request: request,
	}, nil
}

type machineFixtureResolver struct{}

func (machineFixtureResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type selectFirstCandidateStation struct{}

func (*selectFirstCandidateStation) Select(
	_ context.Context,
	call RelevanceCall,
) (RelevanceDecision, error) {
	if len(call.Candidates) != 2 {
		return RelevanceDecision{}, fmt.Errorf("relevance candidates=%d want 2", len(call.Candidates))
	}
	return RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{call.Candidates[0].CandidateID},
		SemanticCalls: 1,
	}, nil
}

type synthesizeFirstEvidenceStation struct{}

func (*synthesizeFirstEvidenceStation) Synthesize(
	_ context.Context,
	call GroundedSynthesisCall,
) (GroundedSynthesisDecision, error) {
	if len(call.Evidence) != 1 {
		return GroundedSynthesisDecision{}, fmt.Errorf("synthesis evidence=%d want 1", len(call.Evidence))
	}
	return GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "The bounded evidence establishes the result.", EvidenceIDs: []EvidenceID{call.Evidence[0].EvidenceID},
	}}, SemanticCalls: 1}, nil
}
