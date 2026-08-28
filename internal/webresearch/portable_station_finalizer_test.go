package webresearch

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPortableStationsFinalizeOnlyAfterTypedLeafValidation(t *testing.T) {
	finalized := 0
	var finalizedErr error
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			candidate := "bounded term"
			if job.Kind == assemblyline.WorkWebSearchTermCoverage {
				var input assemblyline.WebSearchTermLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				candidate = string(assemblyline.WebQueryTermRemains)
				if len(input.AcceptedTerms) > 0 {
					candidate = string(assemblyline.WebNoUncoveredQueryTerm)
				}
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: candidate,
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
	if _, err := station.Resolve(t.Context(), SearchTermsCall{
		Question: "Question", AttemptedQueries: []string{"first query"},
		MaxTerms: 1, MaxTermBytes: 80,
	}); err != nil {
		t.Fatal(err)
	}
	if finalized != 2 || finalizedErr != nil {
		t.Fatalf("finalized=%d validation_error=%v", finalized, finalizedErr)
	}
}

func TestPortableStationsFinalizeTypedLeafRejection(t *testing.T) {
	finalized := 0
	var finalizedErr error
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			candidate := `{"term":"bounded term"}`
			if job.Kind == assemblyline.WorkWebSearchTermCoverage {
				candidate = string(assemblyline.WebQueryTermRemains)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: candidate,
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
	if _, err := station.Resolve(t.Context(), SearchTermsCall{
		Question: "Question", AttemptedQueries: []string{"first query"},
		MaxTerms: 1, MaxTermBytes: 80,
	}); err == nil {
		t.Fatal("invalid typed web leaf was accepted")
	}
	if finalized != 1 || finalizedErr == nil {
		t.Fatalf("finalized=%d validation_error=%v", finalized, finalizedErr)
	}
}

func TestPortableClaimEvidenceReviewFinalizesEverySemanticLeafExactlyOnce(t *testing.T) {
	finalized := 0
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkWebReviewClaimCoverage:
				candidate = string(assemblyline.WebReviewNoUncoveredClaim)
			case assemblyline.WorkWebReviewClaim:
				candidate = "Version 2 is current."
			case assemblyline.WorkWebReviewClaimVerdict:
				candidate = string(assemblyline.WebReviewClaimSupported)
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected review work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
		Finalize: func(_ context.Context, _ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			finalized++
			if validationErr != nil {
				t.Fatalf("validated NONE finalized with error: %v", validationErr)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := station.Review(t.Context(), ClaimEvidenceReviewCall{
		Question: "Which release is current?", ParagraphID: "P1", ParagraphText: "Version 2 is current.",
		Evidence: []ProjectedEvidence{{EvidenceID: "E31", Content: "Version 2 is current."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if finalized != 3 || decision.Outcome != ClaimEvidenceReviewNone || decision.SemanticCalls != 3 {
		t.Fatalf("finalized=%d decision=%+v", finalized, decision)
	}
}

func TestPortableSynthesisCorrectionFinalizesExactlyOnceAfterTypedValidation(t *testing.T) {
	finalized := 0
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: "Version 2 is current.",
			}, nil
		},
		Finalize: func(_ context.Context, _ assemblyline.PortableJob, _ assemblyline.PortableResult, validationErr error) error {
			finalized++
			if validationErr != nil {
				t.Fatalf("validated correction finalized with error: %v", validationErr)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := station.Correct(t.Context(), GroundedSynthesisCorrectionCall{
		Question: "Which release is current?", MaxParagraphBytes: 500,
		Paragraphs: []GroundedParagraph{{Text: "Version 3 is current.", EvidenceIDs: []EvidenceID{"E31"}}},
		Issue: ClaimEvidenceReviewDecision{
			Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1", EvidenceIDs: []EvidenceID{"E31"},
			IssueKind: ClaimEvidenceContradictedSupport, Detail: "The evidence says version 2.",
		},
		Evidence: []ProjectedEvidence{{EvidenceID: "E31", Content: "Version 2 is current."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if finalized != 1 || decision.Text != "Version 2 is current." {
		t.Fatalf("finalized=%d decision=%+v", finalized, decision)
	}
}
