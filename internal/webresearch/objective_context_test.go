package webresearch

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestWebWorkflowProjectsOneUnifiedObjectiveContext(t *testing.T) {
	candidate := candidateFixture("https://context.example/evidence", "Context evidence")
	document := documentFixture(candidate.URL, candidate.Title, "Exact current evidence.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"exact context query": {report: candidateReport("exact context query", candidate)},
		},
		documents: map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	memory := "Exact activated memory."
	feedback := "Preserve the exact question and correct the current generation."
	objectiveContext := assemblyline.ObjectiveContext{
		Capsules: []assemblyline.ObjectiveContextCapsule{{
			Sources: []assemblyline.ObjectiveContextSource{{
				Namespace: "memory", CandidateID: "CTX_1",
				ContentSHA256: assemblyline.ExactObjectiveContextSHA(memory),
			}},
			Content: memory, ContentSHA256: assemblyline.ExactObjectiveContextSHA(memory),
		}},
		ReplanAuthority: &assemblyline.ObjectiveReplanAuthority{
			JobID: 31, Generation: 2, Feedback: feedback,
			FeedbackSHA256: assemblyline.ExactObjectiveContextSHA(feedback),
		},
	}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{
		Paragraphs: []GroundedParagraph{{
			Text: "Exact current evidence.", EvidenceIDs: []EvidenceID{evidenceID(document.ID)},
		}},
	}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_context", Question: "What is current?", Context: objectiveContext,
		InitialQuery: "exact context query", Status: ObjectivePending,
	}, acquisition, relevance, synthesis, 4_000)
	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, projected := range []assemblyline.ObjectiveContext{
		relevance.last.Context, synthesis.last.Context, result.Objective.Context,
	} {
		if !reflect.DeepEqual(projected, objectiveContext) {
			t.Fatalf("web objective context=%+v want %+v", projected, objectiveContext)
		}
	}
}
