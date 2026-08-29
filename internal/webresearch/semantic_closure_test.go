package webresearch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestRelevanceNoneFailsWithoutModelAuthoredQueryExpansion(t *testing.T) {
	initial := candidateFixture("https://irrelevant.example/doc", "Irrelevant")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"initial query": {report: candidateReport("initial query", initial)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			initial.ID: documentFixture(initial.URL, initial.Title, "Unrelated material."),
		},
	}
	relevance := &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceNone, CandidateIDs: []websearch.CandidateID{},
	}}
	synthesis := &recordingSynthesisStation{}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_relevance_retry", Question: "What is the exact answer?", InitialQuery: "initial query",
		Status: ObjectivePending,
	}, acquisition, relevance, synthesis, 2_000)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("Run error=%v want ErrEvidenceUnavailable", err)
	}
	if result.Complete || result.RelevanceCalls != 1 || result.SynthesisCalls != 0 {
		t.Fatalf("result=%#v", result)
	}
	wantEvents := "discover:initial query,fetch:1"
	if got := strings.Join(acquisition.events, ","); got != wantEvents {
		t.Fatalf("events=%q want %q", got, wantEvents)
	}
}

func TestQuestionOverDiscoveryBoundRequiresSeparateExactInitialQuery(t *testing.T) {
	question := strings.Repeat("Which exact current release information is supported? ", 25)
	if len(question) <= 1_024 || len(question) > 4_096 {
		t.Fatalf("fixture question bytes=%d", len(question))
	}
	objective := Objective{
		ID: "objective_long_question", Question: question, InitialQuery: "", Status: ObjectivePending,
	}
	if err := validateObjective(objective); err == nil || !strings.Contains(err.Error(), "initial query") {
		t.Fatalf("missing exact initial query error=%v", err)
	}
}

func TestGroundedSynthesisCompletesThroughCodeOwnedArtifactValidation(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/current", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	evidence := evidenceID(document.ID)
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 2 is current.", EvidenceIDs: []EvidenceID{evidence},
	}}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_grounded", Question: "Which version is current?", InitialQuery: "current version",
		Status: ObjectivePending,
	}, acquisition, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, 2_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Objective.Status != ObjectiveComplete ||
		result.RelevanceCalls != 1 || result.SynthesisCalls != 1 || result.SemanticCalls != 2 {
		t.Fatalf("result=%#v", result)
	}
	if err := ValidateCompletionArtifact(result.Artifact, result.Evidence); err != nil {
		t.Fatalf("completion artifact lost code-owned evidence validation: %v", err)
	}
	wantSteps := []Step{
		StepInitialDiscovery, StepDocumentsFetched, StepRelevanceResolved,
		StepEvidenceProjected, StepSynthesisResolved, StepObjectiveCompleted,
	}
	if !reflect.DeepEqual(result.Steps, wantSteps) {
		t.Fatalf("steps=%v want %v", result.Steps, wantSteps)
	}
}

func TestCanceledContextCannotCommitArtifact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Result{Objective: Objective{Status: ObjectivePending}}
	err := commitCompletion(ctx, &result, Artifact{
		Paragraphs: []GroundedParagraph{{Text: "Claim.", EvidenceIDs: []EvidenceID{"E31"}}},
		Rendered:   "must not commit",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("commit error=%v want context canceled", err)
	}
	if result.Complete || result.Objective.Status == ObjectiveComplete || result.Artifact.Rendered != "" || len(result.Steps) != 0 {
		t.Fatalf("canceled commit mutated completion: %#v", result)
	}
}
