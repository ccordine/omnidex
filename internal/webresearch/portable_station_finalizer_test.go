package webresearch

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPortableStationsFinalizeOnlyAfterTypedLeafValidation(t *testing.T) {
	finalized := 0
	var finalizedErr error
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(assemblyline.WebCandidateRelevant),
			}, nil
		},
		Finalize: func(_ context.Context, _ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			finalized++
			finalizedErr = validationErr
			return nil
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
	if finalized != 1 || finalizedErr != nil {
		t.Fatalf("finalized=%d validation_error=%v", finalized, finalizedErr)
	}
}

func TestPortableStationsFinalizeTypedLeafRejection(t *testing.T) {
	finalized := 0
	var finalizedErr error
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: `{"relation":"RELEVANT"}`,
			}, nil
		},
		Finalize: func(_ context.Context, _ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			finalized++
			finalizedErr = validationErr
			return nil
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
	if finalized != 1 || finalizedErr == nil {
		t.Fatalf("finalized=%d validation_error=%v", finalized, finalizedErr)
	}
}
