package webresearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

type discoverOutcome struct {
	report websearch.CandidateReport
	err    error
}

type scriptedAcquisition struct {
	discoveries map[string]discoverOutcome
	documents   map[websearch.CandidateID]websearch.Document
	events      []string
}

func (*scriptedAcquisition) Limits() websearch.AcquisitionLimits {
	return websearch.AcquisitionLimits{MaxDocuments: 8}
}

func (acquisition *scriptedAcquisition) Discover(_ context.Context, request websearch.QueryRequest) (websearch.CandidateReport, error) {
	acquisition.events = append(acquisition.events, "discover:"+request.Query)
	outcome, ok := acquisition.discoveries[request.Query]
	if !ok {
		return websearch.CandidateReport{}, fmt.Errorf("unscripted discovery %q", request.Query)
	}
	return outcome.report, outcome.err
}

func (acquisition *scriptedAcquisition) Fetch(_ context.Context, request websearch.FetchRequest) (websearch.DocumentReport, error) {
	acquisition.events = append(acquisition.events, fmt.Sprintf("fetch:%d", len(request.CandidateIDs)))
	report := websearch.DocumentReport{}
	for _, id := range request.CandidateIDs {
		document, ok := acquisition.documents[id]
		if !ok {
			report.Diagnostics = append(report.Diagnostics, websearch.DocumentDiagnostic{CandidateID: id, Outcome: websearch.FetchEmpty})
			continue
		}
		report.Documents = append(report.Documents, document)
		report.Diagnostics = append(report.Diagnostics, websearch.DocumentDiagnostic{CandidateID: id, URL: document.URL, Outcome: websearch.FetchSucceeded})
	}
	if len(report.Documents) == 0 {
		return report, websearch.ErrNoDocuments
	}
	return report, nil
}

type cancelingAcquisition struct {
	delegate Acquisition
	cancel   context.CancelFunc
}

func (acquisition *cancelingAcquisition) Limits() websearch.AcquisitionLimits {
	return acquisition.delegate.Limits()
}

func (acquisition *cancelingAcquisition) Discover(ctx context.Context, request websearch.QueryRequest) (websearch.CandidateReport, error) {
	return acquisition.delegate.Discover(ctx, request)
}

func (acquisition *cancelingAcquisition) Fetch(ctx context.Context, request websearch.FetchRequest) (websearch.DocumentReport, error) {
	report, err := acquisition.delegate.Fetch(ctx, request)
	acquisition.cancel()
	return report, err
}

type recordingRelevanceStation struct {
	decision  RelevanceDecision
	decisions []RelevanceDecision
	err       error
	calls     int
	last      RelevanceCall
	events    []string
}

func (station *recordingRelevanceStation) Select(_ context.Context, call RelevanceCall) (RelevanceDecision, error) {
	station.calls++
	station.last = call
	station.events = append(station.events, "relevance")
	if len(station.decisions) > 0 {
		index := station.calls - 1
		if index >= len(station.decisions) {
			index = len(station.decisions) - 1
		}
		decision := station.decisions[index]
		if decision.SemanticCalls == 0 {
			decision.SemanticCalls = 1
		}
		return decision, station.err
	}
	decision := station.decision
	if decision.SemanticCalls == 0 {
		decision.SemanticCalls = 1
	}
	return decision, station.err
}

type recordingSynthesisStation struct {
	decision GroundedSynthesisDecision
	err      error
	calls    int
	last     GroundedSynthesisCall
	events   []string
}

func (station *recordingSynthesisStation) Synthesize(_ context.Context, call GroundedSynthesisCall) (GroundedSynthesisDecision, error) {
	station.calls++
	station.last = call
	station.events = append(station.events, "synthesis")
	decision := station.decision
	if decision.SemanticCalls == 0 {
		decision.SemanticCalls = 1
	}
	return decision, station.err
}

func newFixtureMachine(
	t *testing.T,
	objective Objective,
	acquisition Acquisition,
	relevance RelevanceStation,
	synthesis GroundedSynthesisStation,
	projectionBytes int,
) *Machine {
	t.Helper()
	machine, err := New(objective, Config{
		MaxFetchCandidates:         6,
		MaxProjectionBytes:         projectionBytes,
		MaxRelevantCandidates:      3,
		CandidateSummaryBytes:      240,
		MaxSynthesisParagraphs:     4,
		MaxSynthesisParagraphBytes: 1_000,
	}, acquisition, relevance, synthesis)
	if err != nil {
		t.Fatal(err)
	}
	return machine
}

func candidateFixture(rawURL, title string) websearch.Candidate {
	canonical, err := websearch.CanonicalizeURL(rawURL)
	if err != nil {
		panic(err)
	}
	id, err := websearch.CandidateIDForURL(canonical)
	if err != nil {
		panic(err)
	}
	return websearch.Candidate{
		ID: id, URL: canonical, Title: title, Snippet: title + " summary",
		Sources: []websearch.CandidateSource{{Provider: websearch.ProviderGoogle, SearchURL: "https://www.google.com/search?q=fixture", Rank: 1}},
	}
}

func documentFixture(rawURL, title, content string) websearch.Document {
	candidate := candidateFixture(rawURL, title)
	digest := sha256.Sum256([]byte(strings.TrimSpace(content)))
	contentSHA := hex.EncodeToString(digest[:])
	documentDigest := sha256.Sum256([]byte("web-document.v1\x00" + candidate.URL + "\x00" + contentSHA))
	return websearch.Document{
		ID:          websearch.DocumentID("document_" + hex.EncodeToString(documentDigest[:])),
		CandidateID: candidate.ID, URL: candidate.URL, Title: title, Snippet: title + " summary",
		Content: strings.TrimSpace(content), ContentSHA256: contentSHA,
		ObservedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
	}
}

func candidateReport(query string, candidates ...websearch.Candidate) websearch.CandidateReport {
	return websearch.CandidateReport{
		Query: query, Candidates: candidates,
		Diagnostics: []websearch.ProviderDiagnostic{{Provider: websearch.ProviderGoogle, Outcome: websearch.DiscoverySucceeded, CandidateCount: len(candidates)}},
	}
}

func emptyCandidateReport(query string) websearch.CandidateReport {
	return websearch.CandidateReport{
		Query:       query,
		Diagnostics: []websearch.ProviderDiagnostic{{Provider: websearch.ProviderGoogle, Outcome: websearch.DiscoveryEmpty}},
	}
}
