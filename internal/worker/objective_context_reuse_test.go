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
)

func TestOrdinaryContextStationsRestoreExactLeavesBeforeModelResolution(t *testing.T) {
	t.Parallel()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "Version 2 is the current release.",
	)
	if err != nil {
		t.Fatal(err)
	}
	relevanceInput := assemblyline.ContextRelevanceRelationInput{
		ExactInstruction: "Which release is current?", Candidate: candidate,
		KnownArtifactPaths: []string{},
	}
	minificationInput := assemblyline.ContextMinificationInput{
		ExactInstruction:    "Which release is current?",
		SelectedAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
		KnownArtifactPaths:  []string{},
	}
	reuseCalls := 0
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		reuseCalls++
		response := ""
		switch request.Job.Kind {
		case assemblyline.WorkContextRelevanceRelation:
			if request.Station != station.ContextRelevance {
				t.Fatalf("relevance reuse request=%+v", request)
			}
			response = assemblyline.ContextCandidateDirectlyRelevant
		case assemblyline.WorkContextMinification:
			if request.Station != station.ContextMinification {
				t.Fatalf("minification reuse request=%+v", request)
			}
			response = "Version 2 is current."
		default:
			t.Fatalf("unexpected context reuse work %q", request.Job.Kind)
		}
		projection, err := assemblyline.NewExactPortableResultProjection(response)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID: request.Job.ID, Candidate: response, Projection: &projection,
		}}, true, nil
	}}
	runtime := &nativeRuntimeV3{svc: service, claim: &model.ClaimedStep{}}
	adapter := portableObjectiveContextSieveStations{runtime: runtime}
	relevance, relevanceReceipt, err := adapter.Relate(t.Context(), relevanceInput)
	if err != nil {
		t.Fatal(err)
	}
	minification, minificationReceipt, err := adapter.Minify(t.Context(), minificationInput)
	if err != nil {
		t.Fatal(err)
	}
	if reuseCalls != 2 || relevanceReceipt.Calls != 0 || !relevanceReceipt.Reused ||
		minificationReceipt.Calls != 0 || !minificationReceipt.Reused ||
		relevance.Relation != assemblyline.ContextCandidateDirectlyRelevant ||
		minification.MinimalContext != "Version 2 is current." {
		t.Fatalf(
			"reuse_calls=%d relevance=%+v/%+v minification=%+v/%+v",
			reuseCalls, relevance, relevanceReceipt, minification, minificationReceipt,
		)
	}
}

func TestOrdinaryContextStationRejectsForgedRestoredLeafBeforeModelResolution(t *testing.T) {
	t.Parallel()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "Version 2 is the current release.",
	)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{reuseObjectiveResult: func(
		_ context.Context,
		request queue.ObjectivePortableResultReuseRequest,
	) (queue.ObjectivePortableResultReuse, bool, error) {
		projection, err := assemblyline.NewExactPortableResultProjection(
			assemblyline.ContextCandidateNotDirectlyRelevant,
		)
		if err != nil {
			return queue.ObjectivePortableResultReuse{}, false, err
		}
		return queue.ObjectivePortableResultReuse{Result: assemblyline.PortableResult{
			JobID:      request.Job.ID,
			Candidate:  assemblyline.ContextCandidateDirectlyRelevant,
			Projection: &projection,
		}}, true, nil
	}}
	adapter := portableObjectiveContextSieveStations{runtime: &nativeRuntimeV3{
		svc: service, claim: &model.ClaimedStep{},
	}}
	_, receipt, err := adapter.Relate(t.Context(), assemblyline.ContextRelevanceRelationInput{
		ExactInstruction: "Which release is current?", Candidate: candidate,
		KnownArtifactPaths: []string{},
	})
	if err == nil || receipt.Calls != 0 || receipt.Reused {
		t.Fatalf("forged restored context receipt=%+v error=%v", receipt, err)
	}
}

func TestOrdinaryContextStationSourceHasNoFreshOnlyBypass(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("objective_context_sieve_stations.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "runObjectivePortableRawLeafCall(") ||
		strings.Count(text, "runObjectiveReusablePortableRawLeafCall(") != 2 {
		t.Fatal("ordinary context station bypasses exact accepted-result reuse")
	}
}
