package webresearch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestInvalidAcquisitionReportsCannotReachRelevance(t *testing.T) {
	for _, test := range []struct {
		name   string
		fetch  bool
		mutate func(*acquisitionObservationFixture)
	}{
		{"different query", false, func(f *acquisitionObservationFixture) { f.discovery.Query = "Unrequested subject" }},
		{"missing discovery diagnostic", false, func(f *acquisitionObservationFixture) { f.discovery.Diagnostics = nil }},
		{"empty successful discovery", false, func(f *acquisitionObservationFixture) { f.discovery.Candidates = nil }},
		{"duplicate candidate", false, func(f *acquisitionObservationFixture) {
			f.discovery.Candidates = append(f.discovery.Candidates, f.discovery.Candidates[0])
		}},
		{"candidates on empty discovery", false, func(f *acquisitionObservationFixture) {
			f.after = func(string) error { return websearch.ErrNoCandidates }
		}},
		{"unknown fetched candidate", true, func(f *acquisitionObservationFixture) { f.fetch.Documents[0].CandidateID = "candidate_2" }},
		{"different fetched URL", true, func(f *acquisitionObservationFixture) {
			f.fetch.Documents[0].URL = "https://elsewhere.example/document"
		}},
		{"missing fetch diagnostic", true, func(f *acquisitionObservationFixture) { f.fetch.Diagnostics = nil }},
		{"empty successful fetch", true, func(f *acquisitionObservationFixture) { f.fetch.Documents = nil }},
		{"document on failed diagnostic", true, func(f *acquisitionObservationFixture) { f.fetch.Diagnostics[0].Outcome = websearch.FetchFailed }},
		{"duplicate document", true, func(f *acquisitionObservationFixture) {
			f.fetch.Documents = append(f.fetch.Documents, f.fetch.Documents[0])
		}},
		{"documents on empty fetch", true, func(f *acquisitionObservationFixture) {
			f.after = func(phase string) error {
				if phase == "fetch" {
					return websearch.ErrNoDocuments
				}
				return nil
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAcquisitionObservationFixture("Inspection timetable")
			test.mutate(fixture)
			result, err := GatherRelevantEvidence(context.Background(), EvidenceRequest{
				ID: "invalid-observation", Question: "Inspection timetable", InitialQuery: "Inspection timetable",
			}, EvidenceConfig{MaxFetchCandidates: 1, MaxProjectionBytes: 512, MaxRelevantCandidates: 1, CandidateSummaryBytes: 256}, fixture,
				webUsageRelevanceFunc(func(RelevanceCall) (RelevanceDecision, error) {
					t.Fatal("invalid acquisition reached semantic relevance")
					return RelevanceDecision{}, nil
				}))
			wantFetches := 0
			if test.fetch {
				wantFetches = 1
			}
			if !errors.Is(err, ErrInvalidAcquisition) || fixture.discoveries != 1 || fixture.fetches != wantFetches ||
				len(result.Discovery) != 1 || len(result.Fetches) != wantFetches || len(result.Evidence) != 0 || len(result.Projected) != 0 || result.SemanticCalls != 0 {
				t.Fatalf("invalid report was hidden, retried, or accepted: %+v / %v", result, err)
			}
		})
	}
}

func TestEmptyAcquisitionStopsWithActualDiagnostics(t *testing.T) {
	for _, phase := range []string{"discovery", "fetch"} {
		for _, failed := range []bool{false, true} {
			fixture := newAcquisitionObservationFixture("Publication catalogue")
			wantError := websearch.ErrNoCandidates
			wantFetches := 0
			if phase == "discovery" {
				fixture.discovery.Candidates = nil
				fixture.discovery.Diagnostics[0].CandidateCount = 0
				fixture.discovery.Diagnostics[0].Outcome = websearch.DiscoveryEmpty
				if failed {
					fixture.discovery.Diagnostics[0].Outcome = websearch.DiscoveryFailed
					fixture.discovery.Diagnostics[0].Failure = "HTTP status 503"
				}
			} else {
				wantError, wantFetches = websearch.ErrNoDocuments, 1
				fixture.fetch.Documents = nil
				fixture.fetch.Diagnostics[0].Outcome = websearch.FetchEmpty
				fixture.fetch.Diagnostics[0].Failure = "document contains no normalized text"
				if failed {
					fixture.fetch.Diagnostics[0].Outcome = websearch.FetchFailed
					fixture.fetch.Diagnostics[0].Failure = "HTTP status 503"
				}
			}
			fixture.after = func(at string) error {
				if at == phase {
					return wantError
				}
				return nil
			}
			result, err := gatherAcquisitionObservation(t, context.Background(), fixture)
			if !errors.Is(err, ErrEvidenceUnavailable) || !errors.Is(err, wantError) || len(result.Discovery) != 1 ||
				len(result.Fetches) != wantFetches || fixture.discoveries != 1 || fixture.fetches != wantFetches || len(result.Evidence) != 0 {
				t.Fatalf("empty %s (failed=%t) lost its observation or continued: %+v / %v", phase, failed, result, err)
			}
		}
	}
}

func TestAcquisitionBoundsFailureKeepsTransportAndCancellationErrors(t *testing.T) {
	for _, phase := range []string{"discovery", "fetch"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture := newAcquisitionObservationFixture("Inspection timetable")
			failure := errors.New("observed transport failure")
			if phase == "discovery" {
				fixture.discovery.Diagnostics[0].Failure = strings.Repeat("x", maxAcquisitionDiagnosticBytes+1)
			} else {
				fixture.fetch.Documents[0].Content = strings.Repeat("x", maxAcquisitionContentBytes+1)
			}
			fixture.after = func(at string) error {
				if at == phase {
					cancel()
					return failure
				}
				return nil
			}
			result, err := gatherAcquisitionObservation(t, ctx, fixture)
			if !errors.Is(err, ErrInvalidAcquisition) || !errors.Is(err, context.Canceled) || !errors.Is(err, failure) ||
				len(result.Fetches) != 0 || len(result.Evidence) != 0 || len(result.Projected) != 0 || result.SemanticCalls != 0 {
				t.Fatalf("oversized report was accepted or lost its failure: %+v / %v", result, err)
			}
			if phase == "discovery" && (len(result.Discovery) != 0 || fixture.fetches != 0) ||
				phase == "fetch" && (len(result.Discovery) != 1 || fixture.fetches != 1) || fixture.discoveries != 1 {
				t.Fatalf("bounded acquisition sequence changed: %+v", result)
			}
		})
	}
}
