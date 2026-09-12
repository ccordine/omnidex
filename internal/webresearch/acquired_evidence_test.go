package webresearch

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

type acquiredEvidenceFixture struct {
	discovery            websearch.CandidateReport
	fetch                websearch.DocumentReport
	discoveries, fetches int
}

func (fixture *acquiredEvidenceFixture) Limits() websearch.AcquisitionLimits {
	return websearch.AcquisitionLimits{MaxDocuments: 1}
}

func (fixture *acquiredEvidenceFixture) Discover(_ context.Context, request websearch.QueryRequest) (websearch.CandidateReport, error) {
	fixture.discoveries++
	report := cloneCandidateReport(fixture.discovery)
	report.Query = request.Query
	return report, nil
}

func (fixture *acquiredEvidenceFixture) Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error) {
	fixture.fetches++
	return cloneDocumentReport(fixture.fetch), nil
}

func TestGatheredEvidenceRecordsSourceValuesWithoutChangingThemForProjection(t *testing.T) {
	t.Parallel()
	for _, subject := range []string{"Inspection schedule", "Library classification"} {
		t.Run(subject, func(t *testing.T) {
			content := strings.Repeat(subject+" is documented here. ", 40)
			fixture := &acquiredEvidenceFixture{
				discovery: websearch.CandidateReport{
					Candidates: []websearch.Candidate{{ID: "candidate_1", URL: "https://source.example/document", Title: subject,
						Sources: []websearch.CandidateSource{{Provider: websearch.ProviderBrave, SearchURL: "https://search.brave.com/search?q=fixture", Rank: 1}}}},
					Diagnostics: []websearch.ProviderDiagnostic{{Provider: websearch.ProviderBrave, SearchURL: "https://search.brave.com/search?q=fixture", Outcome: websearch.DiscoverySucceeded, CandidateCount: 1}},
				},
				fetch: websearch.DocumentReport{
					Documents: []websearch.Document{{ID: "document_1", CandidateID: "candidate_1", URL: "https://source.example/document", Title: subject,
						Content: content, ObservedAt: time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)}},
					Diagnostics: []websearch.DocumentDiagnostic{{CandidateID: "candidate_1", URL: "https://source.example/document", Outcome: websearch.FetchSucceeded}},
				},
			}
			semanticCalls := 0
			stations, err := NewPortableStations(PortableRuntime{Resolve: func(_ context.Context, job assemblyline.PortableJob, validate PortableCandidateValidator) (int, error) {
				semanticCalls++
				if job.Kind != assemblyline.WorkWebRelevanceRelation {
					t.Fatalf("unexpected semantic responsibility %s", job.Kind)
				}
				return 1, validate("A")
			}})
			if err != nil {
				t.Fatal(err)
			}
			config := EvidenceConfig{MaxFetchCandidates: 1, MaxRelevantCandidates: 1, MaxProjectionBytes: 256, CandidateSummaryBytes: 256}
			result, err := GatherRelevantEvidence(context.Background(), EvidenceRequest{
				ID: "objective-17-1", Question: "What is documented about this subject?", InitialQuery: subject,
				Context: assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
			}, config, fixture, stations)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Evidence) != 1 || result.Evidence[0].Content != content || result.Evidence[0].Truncated || !result.Projected[0].Truncated {
				t.Fatalf("projection changed the actual source observation: %#v", result)
			}
			captured, err := CaptureEvidence(result, config.MaxProjectionBytes, nil)
			if err != nil {
				t.Fatal(err)
			}
			sources, projected, err := captured.Project()
			if err != nil || !reflect.DeepEqual(sources, result.Evidence) || !reflect.DeepEqual(projected, result.Projected) {
				t.Fatalf("source projection cannot be reconstructed: %#v / %v", captured, err)
			}
			artifact, err := BuildGroundedCompletionArtifact([]GroundedParagraph{{Text: "The source documents the requested subject.", EvidenceIDs: []EvidenceID{sources[0].ID}}}, sources, 1, 1024)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateCompletionArtifact(artifact, sources); err != nil {
				t.Fatal(err)
			}
			artifact.Rendered += " Invented completion."
			if err := ValidateCompletionArtifact(artifact, sources); err == nil {
				t.Fatal("changed completion rendering was accepted")
			}
			result.Fetches[0].Documents[0].Content = "Different fetched value."
			if _, err := CaptureEvidence(result, config.MaxProjectionBytes, nil); err == nil {
				t.Fatal("selected text was accepted against a different fetch observation")
			}
			if captured.Fetch.Documents[0].Content != content {
				t.Fatal("captured source aliases the caller's report")
			}
			for _, mutation := range []func(*AcquiredEvidence){
				func(a *AcquiredEvidence) { a.Fetch.Documents[0].CandidateID = "candidate_2" },
				func(a *AcquiredEvidence) { a.Fetch.Documents = append(a.Fetch.Documents, a.Fetch.Documents[0]) },
				func(a *AcquiredEvidence) { a.Fetch.Diagnostics[0].URL = "https://other.example/document" },
				func(a *AcquiredEvidence) { a.Fetch.Diagnostics[0].Outcome = websearch.FetchFailed },
				func(a *AcquiredEvidence) { a.CandidateIDs[0] = "candidate_2" },
			} {
				raw, err := json.Marshal(captured)
				if err != nil {
					t.Fatal(err)
				}
				var invalid AcquiredEvidence
				if err := json.Unmarshal(raw, &invalid); err != nil {
					t.Fatal(err)
				}
				mutation(&invalid)
				if _, _, err := invalid.Project(); err == nil {
					t.Fatalf("changed acquisition binding was accepted: %#v", invalid)
				}
			}
			if fixture.discoveries != 1 || fixture.fetches != 1 || semanticCalls != 1 {
				t.Fatalf("recording or completion added work: discovery=%d fetch=%d semantic=%d", fixture.discoveries, fixture.fetches, semanticCalls)
			}
		})
	}
}
