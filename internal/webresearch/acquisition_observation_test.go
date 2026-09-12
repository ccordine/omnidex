package webresearch

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestAcquisitionFailureKeepsObservedReportsWithoutAcceptingEvidence(t *testing.T) {
	for _, subject := range []string{"Inspection timetable", "Publication catalogue"} {
		for _, phase := range []string{"discovery", "fetch"} {
			for _, cancelAfterObservation := range []bool{false, true} {
				name := subject + "/" + phase + "/error"
				if cancelAfterObservation {
					name = subject + "/" + phase + "/cancelled"
				}
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					fixture := newAcquisitionObservationFixture(subject)
					wantDiscovery := cloneCandidateReport(fixture.discovery)
					wantFetch := cloneDocumentReport(fixture.fetch)
					failure := errors.New("observed acquisition transport failure")
					fixture.after = func(at string) error {
						if at != phase {
							return nil
						}
						if cancelAfterObservation {
							cancel()
							return nil
						}
						return failure
					}
					result, err := gatherAcquisitionObservation(t, ctx, fixture)
					wantError := failure
					if cancelAfterObservation {
						wantError = context.Canceled
					}
					if !errors.Is(err, wantError) || len(result.Discovery) != 1 || !reflect.DeepEqual(result.Discovery[0], wantDiscovery) {
						t.Fatalf("discovery observation lost: %+v / %v", result, err)
					}
					wantFetches := 0
					if phase == "fetch" {
						wantFetches = 1
					}
					if fixture.discoveries != 1 || fixture.fetches != wantFetches || len(result.Fetches) != wantFetches ||
						len(result.Evidence) != 0 || len(result.Projected) != 0 || result.SemanticCalls != 0 {
						t.Fatalf("failure accepted partial evidence or repeated acquisition: %+v discover=%d fetch=%d / %v", result, fixture.discoveries, fixture.fetches, err)
					}
					if wantFetches == 1 && !reflect.DeepEqual(result.Fetches[0], wantFetch) {
						t.Fatalf("fetch observation lost: %+v", result.Fetches)
					}
					fixture.discovery.Candidates[0].Sources[0].SearchURL = "https://changed.example"
					fixture.fetch.Documents[0].Content = "Changed after acquisition."
					if !reflect.DeepEqual(result.Discovery[0], wantDiscovery) || wantFetches == 1 && !reflect.DeepEqual(result.Fetches[0], wantFetch) {
						t.Fatal("retained observation aliases the acquisition provider")
					}
				})
			}
		}
	}
}

func TestCancelledAcquisitionDoesNotInventReportsOrInvokeProviders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fixture := newAcquisitionObservationFixture("Inspection timetable")
	result, err := gatherAcquisitionObservation(t, ctx, fixture)
	if !errors.Is(err, context.Canceled) || len(result.Discovery) != 0 || len(result.Fetches) != 0 || fixture.discoveries != 0 || fixture.fetches != 0 {
		t.Fatalf("pre-cancelled acquisition ran or invented observations: %+v / %v", result, err)
	}
}

type acquisitionObservationFixture struct {
	discovery            websearch.CandidateReport
	fetch                websearch.DocumentReport
	discoveries, fetches int
	after                func(string) error
}

func (fixture *acquisitionObservationFixture) Limits() websearch.AcquisitionLimits {
	return websearch.AcquisitionLimits{MaxDocuments: 1}
}

func (fixture *acquisitionObservationFixture) Discover(context.Context, websearch.QueryRequest) (websearch.CandidateReport, error) {
	fixture.discoveries++
	return fixture.discovery, fixture.after("discovery")
}

func (fixture *acquisitionObservationFixture) Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error) {
	fixture.fetches++
	return fixture.fetch, fixture.after("fetch")
}

func newAcquisitionObservationFixture(subject string) *acquisitionObservationFixture {
	return &acquisitionObservationFixture{
		discovery: websearch.CandidateReport{
			Query: subject,
			Candidates: []websearch.Candidate{{ID: "candidate_1", URL: "https://source.example/document", Title: subject,
				Sources: []websearch.CandidateSource{{Provider: websearch.ProviderBrave, SearchURL: "https://search.brave.com/search?q=fixture", Rank: 1}}}},
			Diagnostics: []websearch.ProviderDiagnostic{{Provider: websearch.ProviderBrave, SearchURL: "https://search.brave.com/search?q=fixture", Outcome: websearch.DiscoverySucceeded, CandidateCount: 1}},
		},
		fetch: websearch.DocumentReport{
			Documents: []websearch.Document{{ID: "document_1", CandidateID: "candidate_1", URL: "https://source.example/document", Title: subject,
				Content: subject + " is recorded here.", ObservedAt: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}},
			Diagnostics: []websearch.DocumentDiagnostic{{CandidateID: "candidate_1", URL: "https://source.example/document", Outcome: websearch.FetchSucceeded}},
		},
		after: func(string) error { return nil },
	}
}

func gatherAcquisitionObservation(t *testing.T, ctx context.Context, fixture *acquisitionObservationFixture) (EvidenceResult, error) {
	t.Helper()
	return GatherRelevantEvidence(ctx, EvidenceRequest{ID: "observation-fixture", Question: fixture.discovery.Query, InitialQuery: fixture.discovery.Query},
		EvidenceConfig{MaxFetchCandidates: 1, MaxProjectionBytes: 512, MaxRelevantCandidates: 1, CandidateSummaryBytes: 256}, fixture,
		webUsageRelevanceFunc(func(RelevanceCall) (RelevanceDecision, error) {
			t.Fatal("failed acquisition invoked semantic relevance")
			return RelevanceDecision{}, nil
		}))
}
