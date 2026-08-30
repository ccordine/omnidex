package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

func TestWebRelevanceRestoresAcceptedLeafBeforeProviderResolution(t *testing.T) {
	t.Parallel()
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		if request.Job.Kind != assemblyline.WorkWebRelevanceRelation ||
			request.Station != station.WebRelevance {
			t.Fatalf("reuse request=%+v", request)
		}
		candidate := string(assemblyline.WebCandidateRelevant)
		projection, err := assemblyline.NewExactPortableResultProjection(candidate)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: candidate, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	portable, err := webresearch.NewPortableStations(
		runtimeWebPortableRuntime(runtime, station.WebRelevance),
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := portable.Select(t.Context(), webresearch.RelevanceCall{
		Question: "Which release is current?", MaxSelections: 1,
		Candidates: []webresearch.RelevanceCandidate{{
			CandidateID: "C31", Title: "Current", Excerpt: "Version 2 is current.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := decision.CallLedger.ValidateForMaximum("restored web relevance", 1)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 1 || decision.SemanticCalls != 0 || !receipt.Reused ||
		len(decision.CandidateIDs) != 1 || decision.CandidateIDs[0] != "C31" {
		t.Fatalf(
			"reuse_calls=%d receipt=%+v decision=%+v",
			reuseCalls, receipt, decision,
		)
	}
}

func TestWebPortableStationsRestoreCompleteSieveWithoutProviderResolution(t *testing.T) {
	t.Parallel()
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		candidate := ""
		wantStation := station.WebGroundedSynthesis
		switch request.Job.Kind {
		case assemblyline.WorkWebRelevanceRelation:
			candidate = string(assemblyline.WebCandidateRelevant)
			wantStation = station.WebRelevance
		case assemblyline.WorkWebSynthesisParagraphInventory:
			candidate = "Version 2 is current."
		case assemblyline.WorkWebSynthesisParagraphAuthorization:
			candidate = string(assemblyline.WebParagraphResponsiveAndFullySupported)
		case assemblyline.WorkWebSynthesisEvidenceRelation:
			candidate = string(assemblyline.WebEvidenceSupportsParagraph)
		default:
			t.Fatalf("unexpected restored web work %q", request.Job.Kind)
		}
		if request.Station != wantStation {
			t.Fatalf("work=%q station=%q want %q", request.Job.Kind, request.Station, wantStation)
		}
		projection, err := assemblyline.NewExactPortableResultProjection(candidate)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: candidate, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	stations, err := newRoutedWebStations(func(id station.ID) webresearch.PortableRuntime {
		return runtimeWebPortableRuntime(runtime, id)
	})
	if err != nil {
		t.Fatal(err)
	}
	relevance, err := stations.relevance.Select(t.Context(), webresearch.RelevanceCall{
		Question: "Which release is current?", MaxSelections: 1,
		Candidates: []webresearch.RelevanceCandidate{{
			CandidateID: "C31", Title: "Current", Excerpt: "Version 2 is current.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	synthesis, err := stations.synthesis.Synthesize(
		t.Context(), webresearch.GroundedSynthesisCall{
			Question: "Which release is current?", MaxParagraphs: 1,
			MaxParagraphBytes: 500,
			Evidence: []webresearch.ProjectedEvidence{{
				EvidenceID: "E31", CandidateID: "C31", Title: "Current",
				Content: "Version 2 is current.",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var ledger webresearch.SemanticCallLedger
	if err := ledger.Merge("relevance", relevance.CallLedger); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Merge("synthesis", synthesis.CallLedger); err != nil {
		t.Fatal(err)
	}
	receipt, err := ledger.ValidateForMaximum("restored complete web sieve", 4)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 4 || relevance.SemanticCalls != 0 || synthesis.SemanticCalls != 0 ||
		!receipt.Reused || len(synthesis.Paragraphs) != 1 ||
		len(synthesis.Paragraphs[0].EvidenceIDs) != 1 ||
		synthesis.Paragraphs[0].EvidenceIDs[0] != "E31" {
		t.Fatalf(
			"reuse_calls=%d receipt=%+v relevance=%+v synthesis=%+v",
			reuseCalls, receipt, relevance, synthesis,
		)
	}
}

func TestWebReceiptSourceHasNoFreshCallMinimumOrDirectRuntimeBypass(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"objective_web_workflow.go",
		"objective_roleplay_research.go",
		"objective_turn_workflow.go",
		"../webresearch/projection.go",
		"../webresearch/machine.go",
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"SemanticCalls < 1",
			"ModelCalls < 1",
			"runtime.Execute",
			"Execute:",
			"Finalize:",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s contains retired web runtime/minimum %q", path, forbidden)
			}
		}
	}
	runtimeSource, err := os.ReadFile("objective_web_workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(runtimeSource), "runObjectiveReusablePortableRawLeafCall",
	) {
		t.Fatal("web portable runtime bypasses the reusable exact-leaf boundary")
	}
}

func TestWebRelevanceRejectsForgedRestoredLeafBeforeProviderResolution(t *testing.T) {
	t.Parallel()
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		forgedProjection, err := assemblyline.NewExactPortableResultProjection(
			string(assemblyline.WebCandidateNotRelevant),
		)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: string(assemblyline.WebCandidateRelevant),
			Projection: &forgedProjection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	portable, err := webresearch.NewPortableStations(
		runtimeWebPortableRuntime(runtime, station.WebRelevance),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = portable.Select(t.Context(), webresearch.RelevanceCall{
		Question: "Which release is current?", MaxSelections: 1,
		Candidates: []webresearch.RelevanceCandidate{{
			CandidateID: "C31", Title: "Current", Excerpt: "Version 2 is current.",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "validate reused web_") {
		t.Fatalf("forged restored leaf error=%v", err)
	}
}
