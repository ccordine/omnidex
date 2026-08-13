package webresearch

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPortableStationsFinalizeOnlyAfterTypedLeafValidation(t *testing.T) {
	finalized := 0
	var finalizedErr error
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: fmt.Sprintf(`{"schema":%q,"terms":["bounded term"]}`, assemblyline.WebSearchTermsSchemaV1),
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
				JobID:     job.ID,
				Candidate: fmt.Sprintf(`{"schema":%q,"terms":[]}`, assemblyline.WebSearchTermsSchemaV1),
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

func TestPortableClaimEvidenceReviewFinalizesExactlyOnceAfterTypedValidation(t *testing.T) {
	finalized := 0
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID: job.ID,
				Candidate: fmt.Sprintf(
					`{"schema":%q,"outcome":"none","paragraph_id":"","evidence_ids":[],"issue_kind":"","detail":""}`,
					assemblyline.WebClaimEvidenceReviewSchemaV1,
				),
			}, nil
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
	if finalized != 1 || decision.Outcome != ClaimEvidenceReviewNone {
		t.Fatalf("finalized=%d decision=%+v", finalized, decision)
	}
}

func TestPortableSynthesisCorrectionFinalizesExactlyOnceAfterTypedValidation(t *testing.T) {
	finalized := 0
	station, err := NewPortableStations(PortableRuntime{
		Execute: func(_ context.Context, job assemblyline.PortableJob) (assemblyline.PortableResult, error) {
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: `{"text":"Version 2 is current."}`,
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
