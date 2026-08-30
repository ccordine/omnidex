package webresearch

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPortableStationsResolveOnlyAfterTypedLeafValidation(t *testing.T) {
	validated := 0
	station, err := NewPortableStations(PortableRuntime{
		Resolve: func(
			_ context.Context,
			_ assemblyline.PortableJob,
			validate PortableCandidateValidator,
		) (SemanticCallReceipt, error) {
			validated++
			return SemanticCallReceipt{Calls: 1}, validate(
				string(assemblyline.WebCandidateRelevant),
			)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := station.Select(t.Context(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{CandidateID: "C1", Excerpt: "Evidence"}},
	}); err != nil {
		t.Fatal(err)
	}
	if validated != 1 {
		t.Fatalf("typed validations=%d", validated)
	}
}

func TestPortableStationsResolvePropagatesTypedLeafRejection(t *testing.T) {
	validated := 0
	var validationErr error
	station, err := NewPortableStations(PortableRuntime{
		Resolve: func(
			_ context.Context,
			_ assemblyline.PortableJob,
			validate PortableCandidateValidator,
		) (SemanticCallReceipt, error) {
			validated++
			validationErr = validate(`{"relation":"RELEVANT"}`)
			return SemanticCallReceipt{Calls: 1}, validationErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := station.Select(t.Context(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{CandidateID: "C1", Excerpt: "Evidence"}},
	}); err == nil {
		t.Fatal("invalid typed web leaf was accepted")
	}
	if validated != 1 || validationErr == nil {
		t.Fatalf("typed validations=%d validation_error=%v", validated, validationErr)
	}
}
