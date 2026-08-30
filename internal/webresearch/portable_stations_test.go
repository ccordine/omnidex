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

func testPortableRuntime(execute func(
	context.Context,
	assemblyline.PortableJob,
) (assemblyline.PortableResult, error)) PortableRuntime {
	return PortableRuntime{
		Resolve: func(
			ctx context.Context,
			job assemblyline.PortableJob,
			validate PortableCandidateValidator,
		) (SemanticCallReceipt, error) {
			result, err := execute(ctx, job)
			receipt := SemanticCallReceipt{Calls: exactPortableSemanticLeafCalls}
			if err != nil {
				return receipt, err
			}
			if err := result.ValidateFor(job); err != nil {
				return receipt, err
			}
			return receipt, validate(result.Candidate)
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
		case assemblyline.WorkWebSynthesisParagraphInventory:
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
		case assemblyline.WorkWebSynthesisParagraphAuthorization:
			candidate = string(assemblyline.WebParagraphResponsiveAndFullySupported)
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
		assemblyline.WorkWebSynthesisParagraphInventory,
		assemblyline.WorkWebSynthesisParagraphAuthorization,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisEvidenceRelation,
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

func TestPortableSynthesisCandidateQueueDiscardsRejectedAndNeverRechecksAccepted(t *testing.T) {
	const (
		accepted    = "Version 2 is current."
		unsupported = "Version 3 is current."
		invented    = "Version 2 is current and its maintainer is Alice."
	)
	workKinds := make([]assemblyline.WorkKind, 0, 9)
	supportCalls := make(map[string]int)
	authorizationCalls := make(map[string]int)
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		workKinds = append(workKinds, job.Kind)
		var candidate string
		switch job.Kind {
		case assemblyline.WorkWebSynthesisParagraphInventory:
			var input assemblyline.WebGroundedSynthesisInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			if input.ExactQuestion != "Which release is current?" {
				return assemblyline.PortableResult{}, fmt.Errorf("inventory question=%q", input.ExactQuestion)
			}
			candidate = strings.Join([]string{accepted, unsupported, invented, accepted}, "\n")
		case assemblyline.WorkWebSynthesisEvidenceRelation:
			var input assemblyline.WebSynthesisEvidenceRelationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			if input.ExactQuestion != "Which release is current?" {
				return assemblyline.PortableResult{}, fmt.Errorf("support question=%q", input.ExactQuestion)
			}
			supportCalls[input.ParagraphText]++
			candidate = string(assemblyline.WebEvidenceDoesNotSupport)
			if input.Evidence.EvidenceID == "E31" && input.ParagraphText != unsupported {
				candidate = string(assemblyline.WebEvidenceSupportsParagraph)
			}
		case assemblyline.WorkWebSynthesisParagraphAuthorization:
			var input assemblyline.WebSynthesisParagraphAuthorizationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			if input.ExactQuestion != "Which release is current?" ||
				len(input.Evidence) != 2 || input.Evidence[0].EvidenceID != "E17" ||
				input.Evidence[1].EvidenceID != "E31" {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"authorization authority question=%q evidence=%+v",
					input.ExactQuestion, input.Evidence,
				)
			}
			authorizationCalls[input.ParagraphText]++
			candidate = string(assemblyline.WebParagraphNotResponsiveOrUnsupported)
			if input.ParagraphText == accepted {
				candidate = string(assemblyline.WebParagraphResponsiveAndFullySupported)
			}
		default:
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected job %q", job.Kind)
		}
		return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	decision, err := station.Synthesize(context.Background(), GroundedSynthesisCall{
		Question: "Which release is current?", MaxParagraphs: 4, MaxParagraphBytes: 500,
		Evidence: []ProjectedEvidence{
			{EvidenceID: "E17", Title: "History", Content: "Version 1 was superseded."},
			{EvidenceID: "E31", Title: "Current", Content: "Version 2 is current."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Paragraphs) != 1 || decision.Paragraphs[0].Text != accepted {
		t.Fatalf("decision=%+v", decision)
	}
	if fmt.Sprint(decision.Paragraphs[0].EvidenceIDs) != "[E31]" {
		t.Fatalf("accepted evidence IDs=%v", decision.Paragraphs[0].EvidenceIDs)
	}
	if decision.SemanticCalls != 6 || len(workKinds) != 6 {
		t.Fatalf("semantic calls=%d work=%v", decision.SemanticCalls, workKinds)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkWebSynthesisParagraphInventory,
		assemblyline.WorkWebSynthesisParagraphAuthorization,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisEvidenceRelation,
		assemblyline.WorkWebSynthesisParagraphAuthorization,
		assemblyline.WorkWebSynthesisParagraphAuthorization,
	}
	if fmt.Sprint(workKinds) != fmt.Sprint(wantKinds) {
		t.Fatalf("work kinds=%v want %v", workKinds, wantKinds)
	}
	if supportCalls[accepted] != 2 || authorizationCalls[accepted] != 1 {
		t.Fatalf(
			"accepted candidate was not processed exactly once: support=%d authorization=%d",
			supportCalls[accepted], authorizationCalls[accepted],
		)
	}
	if supportCalls[unsupported] != 0 || authorizationCalls[unsupported] != 1 {
		t.Fatalf(
			"unsupported candidate did not evaporate before pairwise evidence work: support=%d authorization=%d",
			supportCalls[unsupported], authorizationCalls[unsupported],
		)
	}
	if supportCalls[invented] != 0 || authorizationCalls[invented] != 1 {
		t.Fatalf(
			"rejected candidate did not evaporate after exactly one authorization: support=%d authorization=%d",
			supportCalls[invented], authorizationCalls[invented],
		)
	}
}

func TestPortableSynthesisRegisteredInventoryAbsenceStopsWithoutMoreModelWork(t *testing.T) {
	calls := 0
	station, err := NewPortableStations(testPortableRuntime(func(
		_ context.Context,
		job assemblyline.PortableJob,
	) (assemblyline.PortableResult, error) {
		calls++
		if job.Kind != assemblyline.WorkWebSynthesisParagraphInventory {
			return assemblyline.PortableResult{}, fmt.Errorf("unexpected job %q", job.Kind)
		}
		return assemblyline.PortableResult{
			JobID: job.ID, Candidate: assemblyline.WebNoSynthesisParagraphCandidates,
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = station.Synthesize(context.Background(), GroundedSynthesisCall{
		Question: "Which release is current?", MaxParagraphs: 2, MaxParagraphBytes: 500,
		Evidence: []ProjectedEvidence{{
			EvidenceID: "E31", Title: "Current", Content: "Version 2 is current.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "queue produced no responsive fully supported paragraphs") {
		t.Fatalf("error=%v", err)
	}
	if calls != 1 {
		t.Fatalf("registered absence triggered %d semantic calls", calls)
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
