package webresearch

import (
	"context"
	"encoding/json"
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

func TestPortableStationsExecuteOneJobPerSemanticLeaf(t *testing.T) {
	calls := make([]assemblyline.WorkKind, 0, 12)
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls = append(calls, job.Kind)
		if _, err := assemblyline.RenderPortableJob(job); err != nil {
			return assemblyline.PortableResult{}, err
		}
		candidate := ""
		switch job.Kind {
		case assemblyline.WorkWebSearchTermCoverage:
			var input assemblyline.WebSearchTermLeafInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.WebQueryTermRemains)
			if len(input.AcceptedTerms) > 0 {
				candidate = string(assemblyline.WebNoUncoveredQueryTerm)
			}
		case assemblyline.WorkWebSearchTerm:
			candidate = "stable release"
		case assemblyline.WorkWebRelevanceRelation:
			var input assemblyline.WebRelevanceRelationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.WebCandidateNotRelevant)
			if input.Candidate.CandidateID == "C31" {
				candidate = string(assemblyline.WebCandidateRelevant)
			}
		case assemblyline.WorkWebSynthesisParagraphCoverage:
			var input assemblyline.WebSynthesisParagraphLeafInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.WebSynthesisParagraphRemains)
			if len(input.AcceptedParagraphs) > 0 {
				candidate = string(assemblyline.WebSynthesisNoUncoveredParagraph)
			}
		case assemblyline.WorkWebSynthesisParagraph:
			candidate = "Version 2 is current."
		case assemblyline.WorkWebSynthesisEvidenceRelation:
			var input assemblyline.WebSynthesisEvidenceRelationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.WebEvidenceDoesNotSupport)
			if input.Evidence.EvidenceID == "E31" {
				candidate = string(assemblyline.WebEvidenceSupportsParagraph)
			}
		case assemblyline.WorkWebGroundedSynthesisCorrection:
			candidate = "Version 2 is current."
		case assemblyline.WorkWebReviewClaimCoverage:
			var input assemblyline.WebReviewClaimLeafInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			candidate = string(assemblyline.WebReviewClaimRemains)
			if len(input.AcceptedClaims) > 0 {
				candidate = string(assemblyline.WebReviewNoUncoveredClaim)
			}
		case assemblyline.WorkWebReviewClaim:
			candidate = "Version 2 is current."
		case assemblyline.WorkWebReviewClaimVerdict:
			candidate = string(assemblyline.WebReviewClaimSupported)
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
		assemblyline.WorkWebSearchTerm,
		assemblyline.WorkWebSearchTermCoverage,
		assemblyline.WorkWebRelevanceRelation,
		assemblyline.WorkWebRelevanceRelation,
		assemblyline.WorkWebSynthesisParagraph,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisParagraphCoverage,
		assemblyline.WorkWebGroundedSynthesisCorrection,
		assemblyline.WorkWebReviewClaim,
		assemblyline.WorkWebReviewClaimVerdict,
		assemblyline.WorkWebReviewClaimCoverage,
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("calls=%v want %v", calls, want)
	}
	if terms.SemanticCalls != 2 || relevance.SemanticCalls != 2 ||
		synthesis.SemanticCalls != 4 || correction.SemanticCalls != 1 || review.SemanticCalls != 3 {
		t.Fatalf(
			"semantic calls terms=%d relevance=%d synthesis=%d correction=%d review=%d",
			terms.SemanticCalls, relevance.SemanticCalls, synthesis.SemanticCalls,
			correction.SemanticCalls, review.SemanticCalls,
		)
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
			JobID:     job.ID,
			Candidate: `{"relation":"RELEVANT"}`,
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

func TestPortableReviewAssemblesOneIssueFromFourIndependentLeaves(t *testing.T) {
	calls := make([]assemblyline.WorkKind, 0, 4)
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls = append(calls, job.Kind)
		candidate := ""
		switch job.Kind {
		case assemblyline.WorkWebReviewClaimCoverage:
			candidate = string(assemblyline.WebReviewClaimRemains)
		case assemblyline.WorkWebReviewClaim:
			candidate = "Version 3 is current."
		case assemblyline.WorkWebReviewClaimVerdict:
			candidate = string(assemblyline.WebReviewClaimContradicted)
		case assemblyline.WorkWebReviewIssueEvidenceRelation:
			candidate = string(assemblyline.WebReviewEvidenceImplicated)
		case assemblyline.WorkWebReviewIssueDetail:
			candidate = "The evidence identifies version 2 as current."
		default:
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected job %q", job.Kind)
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	decision, err := station.Review(t.Context(), ClaimEvidenceReviewCall{
		Question: "Which release is current?", ParagraphID: "P1",
		ParagraphText: "Version 3 is current.",
		Evidence:      []ProjectedEvidence{{EvidenceID: "E31", Content: "Version 2 is current."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Outcome != ClaimEvidenceReviewIssue || decision.ParagraphID != "P1" ||
		len(decision.EvidenceIDs) != 1 || decision.EvidenceIDs[0] != "E31" ||
		decision.IssueKind != ClaimEvidenceContradictedSupport || decision.SemanticCalls != 4 {
		t.Fatalf("decision=%+v", decision)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkWebReviewClaim,
		assemblyline.WorkWebReviewClaimVerdict,
		assemblyline.WorkWebReviewIssueEvidenceRelation,
		assemblyline.WorkWebReviewIssueDetail,
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("calls=%v want %v", calls, want)
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
