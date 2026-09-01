package webresearch

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestPortableEvidenceSelectionAggregatesIndependentBinaryLeaves(t *testing.T) {
	t.Parallel()
	responses := map[string]string{
		"candidate-1": "A",
		"candidate-2": "B",
		"candidate-3": "A",
	}
	seen := make([]string, 0, len(responses))
	stations, err := NewPortableStations(PortableRuntime{
		Resolve: func(
			_ context.Context,
			job assemblyline.PortableJob,
			validate PortableCandidateValidator,
		) (SemanticCallReceipt, error) {
			if job.Kind != assemblyline.WorkWebRelevanceRelation {
				t.Fatalf("work kind=%q", job.Kind)
			}
			var input assemblyline.WebRelevanceRelationInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			seen = append(seen, input.Candidate.CandidateID)
			receipt := SemanticCallReceipt{Calls: exactPortableSemanticLeafCalls}
			return receipt, validate(responses[input.Candidate.CandidateID])
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	decision, err := stations.Select(context.Background(), portableEvidenceSelectionCall())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"candidate-1", "candidate-2", "candidate-3"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("binary leaf order=%v, want %v", seen, want)
	}
	if want := []websearch.CandidateID{"candidate-1", "candidate-3"}; !reflect.DeepEqual(decision.CandidateIDs, want) {
		t.Fatalf("selected IDs=%v, want %v", decision.CandidateIDs, want)
	}
	if decision.Outcome != RelevanceSelected || decision.SemanticCalls != 3 ||
		decision.CallLedger.Count() != 3 {
		t.Fatalf("decision provenance=%+v", decision)
	}
}

func TestPortableEvidenceSelectionRejectsAggregateModelPackets(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`["candidate-1","candidate-3"]`,
		`{"candidate_ids":["candidate-1","candidate-3"]}`,
	} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			calls := 0
			stations, err := NewPortableStations(PortableRuntime{
				Resolve: func(
					_ context.Context,
					_ assemblyline.PortableJob,
					validate PortableCandidateValidator,
				) (SemanticCallReceipt, error) {
					calls++
					receipt := SemanticCallReceipt{Calls: exactPortableSemanticLeafCalls}
					return receipt, validate(raw)
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stations.Select(
				context.Background(), portableEvidenceSelectionCall(),
			); err == nil {
				t.Fatal("aggregate model response was decoded as web evidence selection")
			}
			if calls != 1 {
				t.Fatalf("provider calls=%d, want first binary leaf only", calls)
			}
		})
	}
}

func portableEvidenceSelectionCall() RelevanceCall {
	return RelevanceCall{
		Question: "Which evidence directly answers when the inspection occurs?",
		Context:  assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{}},
		Candidates: []RelevanceCandidate{
			{CandidateID: "candidate-1", Title: "Schedule", Snippet: "Inspection timing", Excerpt: "The inspection occurs Monday."},
			{CandidateID: "candidate-2", Title: "Paint", Snippet: "Available colors", Excerpt: "The frame is blue."},
			{CandidateID: "candidate-3", Title: "Calendar", Snippet: "Confirmed date", Excerpt: "Monday is the confirmed inspection date."},
		},
		MaxSelections: 3,
	}
}
