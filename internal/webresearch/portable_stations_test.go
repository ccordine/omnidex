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
	_ RelevanceStation         = (*PortableStations)(nil)
	_ GroundedSynthesisStation = (*PortableStations)(nil)
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
		default:
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected job %q", job.Kind)
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	question := "  Which release is current?\n"
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
	want := []assemblyline.WorkKind{
		assemblyline.WorkWebRelevanceRelation,
		assemblyline.WorkWebRelevanceRelation,
		assemblyline.WorkWebSynthesisParagraph,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisParagraphCoverage,
	}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("calls=%v want %v", calls, want)
	}
	if relevance.SemanticCalls != 2 || synthesis.SemanticCalls != 4 {
		t.Fatalf(
			"semantic calls relevance=%d synthesis=%d",
			relevance.SemanticCalls, synthesis.SemanticCalls,
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
	_, err = station.Select(context.Background(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{CandidateID: "C31", Excerpt: "Evidence"}},
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
	_, err := station.Select(context.Background(), RelevanceCall{
		Question: "Question", MaxSelections: 1,
		Candidates: []RelevanceCandidate{{CandidateID: "C31", Excerpt: "Evidence"}},
	})
	if err == nil {
		t.Fatal("uninitialized stations did not fail loudly")
	}
}
