package webresearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

var (
	_ SearchTermsStation                 = (*PortableStations)(nil)
	_ RelevanceStation                   = (*PortableStations)(nil)
	_ GroundedSynthesisStation           = (*PortableStations)(nil)
	_ GroundedSynthesisCorrectionStation = (*PortableStations)(nil)
	_ ClaimEvidenceReviewStation         = (*PortableStations)(nil)
)

func testPortableRuntime(execute PortableExecutor) PortableRuntime {
	return PortableRuntime{
		Execute: execute,
		Finalize: func(context.Context, assemblyline.PortableJob, assemblyline.PortableResult, error) error {
			return nil
		},
	}
}

func TestPortableStationsExecuteFiveExactLeafJobs(t *testing.T) {
	calls := make([]assemblyline.WorkKind, 0, 5)
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls = append(calls, job.Kind)
		if _, _, err := assemblyline.RenderPortableJob(job); err != nil {
			return assemblyline.PortableResult{}, err
		}
		candidate := ""
		switch job.Kind {
		case assemblyline.WorkWebSearchTerms:
			candidate = fmt.Sprintf(`{"schema":%q,"terms":["stable release"]}`, assemblyline.WebSearchTermsSchemaV1)
		case assemblyline.WorkWebRelevance:
			candidate = fmt.Sprintf(`{"schema":%q,"candidate_ids":["C31"]}`, assemblyline.WebRelevanceSchemaV1)
		case assemblyline.WorkWebGroundedSynthesis:
			candidate = fmt.Sprintf(`{"schema":%q,"paragraphs":[{"text":"Version 2 is current.","evidence_ids":["E31"]}]}`, assemblyline.WebGroundedSynthesisSchemaV1)
		case assemblyline.WorkWebGroundedSynthesisCorrection:
			candidate = `{"text":"Version 2 is current."}`
		case assemblyline.WorkWebClaimEvidenceReview:
			candidate = fmt.Sprintf(`{"schema":%q,"outcome":"none"}`, assemblyline.WebClaimEvidenceReviewSchemaV1)
		default:
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected job %q", job.Kind)
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	question := "  Which release is current?\n"
	terms, err := station.Resolve(context.Background(), SearchTermsCall{
		Question: question, AttemptedQueries: []string{"current release"},
		MaxTerms: 2, MaxTermBytes: 80,
	})
	if err != nil || len(terms.Terms) != 1 || terms.Terms[0] != "stable release" {
		t.Fatalf("terms=%+v err=%v", terms, err)
	}
	relevance, err := station.Select(context.Background(), RelevanceCall{
		Question: question, MaxSelections: 1,
		Candidates: []RelevanceCandidate{
			{CandidateID: websearch.CandidateID("C17"), Title: "Old", Excerpt: "Old release."},
			{CandidateID: websearch.CandidateID("C31"), Title: "Current", Excerpt: "Current release."},
		},
	})
	if err != nil || len(relevance.CandidateIDs) != 1 || relevance.CandidateIDs[0] != "C31" {
		t.Fatalf("relevance=%+v err=%v", relevance, err)
	}
	synthesis, err := station.Synthesize(context.Background(), GroundedSynthesisCall{
		Question: question, MaxParagraphs: 2, MaxParagraphBytes: 500,
		Evidence: []ProjectedEvidence{
			{EvidenceID: "E17", Title: "Old", Content: "Version 1 was superseded."},
			{EvidenceID: "E31", Title: "Current", Content: "Version 2 is current."},
		},
	})
	if err != nil || len(synthesis.Paragraphs) != 1 || synthesis.Paragraphs[0].EvidenceIDs[0] != "E31" {
		t.Fatalf("synthesis=%+v err=%v", synthesis, err)
	}
	correction, err := station.Correct(context.Background(), GroundedSynthesisCorrectionCall{
		Question: question, MaxParagraphBytes: 500,
		Paragraphs: []GroundedParagraph{{Text: "Version 3 is current.", EvidenceIDs: []EvidenceID{"E31"}}},
		Issue: ClaimEvidenceReviewDecision{
			Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1", EvidenceIDs: []EvidenceID{"E31"},
			IssueKind: ClaimEvidenceContradictedSupport, Detail: "The evidence says version 2.",
		},
		Evidence: []ProjectedEvidence{{EvidenceID: "E31", Title: "Current", Content: "Version 2 is current."}},
	})
	if err != nil || correction.Text != "Version 2 is current." {
		t.Fatalf("correction=%+v err=%v", correction, err)
	}
	review, err := station.Review(context.Background(), ClaimEvidenceReviewCall{
		Question: question, ParagraphID: "P1", ParagraphText: "Version 2 is current.",
		Evidence: []ProjectedEvidence{{EvidenceID: "E31", Title: "Current", Content: "Version 2 is current."}},
	})
	if err != nil || review.Outcome != ClaimEvidenceReviewNone || review.EvidenceIDs == nil {
		t.Fatalf("review=%+v err=%v", review, err)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkWebSearchTerms,
		assemblyline.WorkWebRelevance,
		assemblyline.WorkWebGroundedSynthesis,
		assemblyline.WorkWebGroundedSynthesisCorrection,
		assemblyline.WorkWebClaimEvidenceReview,
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("calls=%v want %v", calls, want)
	}
}

func TestPortableStationsRejectInvalidResultWithoutFallback(t *testing.T) {
	calls := 0
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls++
		return assemblyline.PortableResult{
			JobID: job.ID,
			Candidate: fmt.Sprintf(
				`{"schema":%q,"candidate_ids":["C99"]}`,
				assemblyline.WebRelevanceSchemaV1,
			),
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = station.Select(context.Background(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{CandidateID: "C31", Excerpt: "Evidence"}},
	})
	if err == nil {
		t.Fatal("out-of-set model ID was accepted")
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d; invalid result triggered a fallback", calls)
	}
}

func TestPortableStationsFailBeforeExecutorForOversizedProjection(t *testing.T) {
	calls := 0
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls++
		return assemblyline.PortableResult{JobID: job.ID, Candidate: `{}`}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = station.Select(context.Background(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{
			CandidateID: "C31", Excerpt: strings.Repeat("x", maxPortableCandidateFieldBytes+1),
		}},
	})
	if err == nil {
		t.Fatal("oversized relevance projection was accepted")
	}
	_, synthesisErr := station.Synthesize(context.Background(), GroundedSynthesisCall{
		Question: "Question", MaxParagraphs: 1, MaxParagraphBytes: 500,
		Evidence: []ProjectedEvidence{{
			EvidenceID: "E31", Content: strings.Repeat("x", maxPortableEvidenceProjection+1),
		}},
	})
	if synthesisErr == nil {
		t.Fatal("oversized synthesis projection was accepted")
	}
	if calls != 0 {
		t.Fatalf("executor calls=%d before bounded input validation", calls)
	}
}

func TestPortableStationsPropagateExecutorFailureOnce(t *testing.T) {
	calls := 0
	want := errors.New("model unavailable")
	station, err := NewPortableStations(testPortableRuntime(func(
		context.Context,
		assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls++
		return assemblyline.PortableResult{}, want
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = station.Resolve(context.Background(), SearchTermsCall{
		Question: "Question", AttemptedQueries: []string{"query"},
		MaxTerms: 1, MaxTermBytes: 80,
	})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d; failure triggered fallback", calls)
	}
}

func TestPortableStationsRequireInitializedBoundary(t *testing.T) {
	if _, err := NewPortableStations(PortableRuntime{}); err == nil {
		t.Fatal("nil portable executor was accepted")
	}
	var station *PortableStations
	_, err := station.Resolve(context.Background(), SearchTermsCall{
		Question: "Question", AttemptedQueries: []string{"query"},
		MaxTerms: 1, MaxTermBytes: 80,
	})
	if err == nil {
		t.Fatal("uninitialized stations did not fail loudly")
	}
}
