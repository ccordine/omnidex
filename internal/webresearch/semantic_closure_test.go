package webresearch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestRelevanceNoneUsesRemainingBoundedSearchTermExpansion(t *testing.T) {
	initial := candidateFixture("https://irrelevant.example/doc", "Irrelevant")
	relevant := candidateFixture("https://relevant.example/doc", "Relevant")
	relevantDocument := documentFixture(relevant.URL, relevant.Title, "The exact answer is supported here.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"initial query": {report: candidateReport("initial query", initial)},
			"focused query": {report: candidateReport("focused query", relevant)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			initial.ID:  documentFixture(initial.URL, initial.Title, "Unrelated material."),
			relevant.ID: relevantDocument,
		},
	}
	relevance := &recordingRelevanceStation{decisions: []RelevanceDecision{
		{Outcome: RelevanceNone, CandidateIDs: []websearch.CandidateID{}},
		{Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{relevant.ID}},
	}}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "The exact answer is supported.", EvidenceIDs: []EvidenceID{evidenceID(relevantDocument.ID)},
	}}}}
	review := &recordingClaimEvidenceReviewStation{}
	machine := newFixtureMachineWithReview(t, Objective{
		ID: "objective_relevance_retry", Question: "What is the exact answer?", InitialQuery: "initial query",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"focused query"}}},
		relevance, synthesis, review, 2_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SearchTermsCalls != 1 || result.RelevanceCalls != 2 ||
		result.SynthesisCalls != 1 || result.ClaimEvidenceReviewCalls != 1 {
		t.Fatalf("result=%#v", result)
	}
	wantEvents := "discover:initial query,fetch:1,discover:focused query,fetch:1"
	if got := strings.Join(acquisition.events, ","); got != wantEvents {
		t.Fatalf("events=%q want %q", got, wantEvents)
	}
}

func TestQuestionOverDiscoveryBoundOpensSearchTermGapBeforeDiscover(t *testing.T) {
	question := strings.Repeat("Which exact current release information is supported? ", 25)
	if len(question) <= 1_024 || len(question) > 4_096 {
		t.Fatalf("fixture question bytes=%d", len(question))
	}
	candidate := candidateFixture("https://current.example/release", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current release": {report: candidateReport("current release", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	terms := &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"current release"}}}
	machine := newFixtureMachine(t, Objective{
		ID: "objective_long_question", Question: question, InitialQuery: "",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, terms, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 2 is current.", EvidenceIDs: []EvidenceID{evidenceID(document.ID)},
	}}}}, 2_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || terms.calls != 1 || terms.last.Question != question ||
		terms.last.AttemptedQueries == nil || len(terms.last.AttemptedQueries) != 0 {
		t.Fatalf("long question path result=%#v terms=%#v", result, terms.last)
	}
	if got := strings.Join(acquisition.events, ","); got != "discover:current release,fetch:1" {
		t.Fatalf("oversized exact question reached discovery: %q", got)
	}
}

func TestSecondRelevanceNoneFailsWithoutSynthesisOrCompletion(t *testing.T) {
	initial := candidateFixture("https://first.example/doc", "First")
	second := candidateFixture("https://second.example/doc", "Second")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{
			"first query":  {report: candidateReport("first query", initial)},
			"second query": {report: candidateReport("second query", second)},
		},
		documents: map[websearch.CandidateID]websearch.Document{
			initial.ID: documentFixture(initial.URL, initial.Title, "Unrelated first material."),
			second.ID:  documentFixture(second.URL, second.Title, "Unrelated second material."),
		},
	}
	relevance := &recordingRelevanceStation{decisions: []RelevanceDecision{
		{Outcome: RelevanceNone, CandidateIDs: []websearch.CandidateID{}},
		{Outcome: RelevanceNone, CandidateIDs: []websearch.CandidateID{}},
	}}
	synthesis := &recordingSynthesisStation{}
	review := &recordingClaimEvidenceReviewStation{}
	machine := newFixtureMachineWithReview(t, Objective{
		ID: "objective_no_evidence", Question: "What is supported?", InitialQuery: "first query",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{decision: SearchTermsDecision{Terms: []string{"second query"}}},
		relevance, synthesis, review, 2_000)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrEvidenceUnavailable) {
		t.Fatalf("Run error=%v want ErrEvidenceUnavailable", err)
	}
	if result.Complete || result.Objective.Status == ObjectiveComplete || synthesis.calls != 0 || review.calls != 0 {
		t.Fatalf("failed relevance synthesized or completed: result=%#v synthesis=%d review=%d", result, synthesis.calls, review.calls)
	}
}

func TestIndependentClaimEvidenceIssuePreventsCompletion(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/current", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 3 is current.", EvidenceIDs: []EvidenceID{evidenceID(document.ID)},
	}}}}
	review := &recordingClaimEvidenceReviewStation{decisions: []ClaimEvidenceReviewDecision{{
		Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1",
		EvidenceIDs: []EvidenceID{evidenceID(document.ID)}, IssueKind: ClaimEvidenceContradictedSupport,
		Detail: "The cited evidence says version 2, not version 3.",
	}}}
	correction := &recordingSynthesisCorrectionStation{decision: GroundedSynthesisCorrectionDecision{Text: "Version 4 is current."}}
	machine := newFixtureMachineWithCorrection(t, Objective{
		ID: "objective_review_issue", Question: "Which version is current?", InitialQuery: "current version",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, correction, review, 2_000)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrClaimEvidenceInadequate) {
		t.Fatalf("Run error=%v want ErrClaimEvidenceInadequate", err)
	}
	if result.Complete || result.Objective.Status == ObjectiveComplete || result.Artifact.Rendered != "" ||
		review.calls != 2 || correction.calls != 1 {
		t.Fatalf("review issue completed objective: %#v calls=%d", result, review.calls)
	}
}

func TestClaimEvidenceIssueUsesOneCorrectionThenIndependentReReview(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/corrected", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	evidence := evidenceID(document.ID)
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 3 is current.", EvidenceIDs: []EvidenceID{evidence},
	}}}}
	correction := &recordingSynthesisCorrectionStation{decision: GroundedSynthesisCorrectionDecision{Text: "Version 2 is current."}}
	review := &recordingClaimEvidenceReviewStation{decisions: []ClaimEvidenceReviewDecision{
		{Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1", EvidenceIDs: []EvidenceID{evidence},
			IssueKind: ClaimEvidenceContradictedSupport, Detail: "The cited evidence says version 2, not version 3."},
		{Outcome: ClaimEvidenceReviewNone, EvidenceIDs: []EvidenceID{}},
	}}
	machine := newFixtureMachineWithCorrection(t, Objective{
		ID: "objective_corrected_review", Question: "Which version is current?", InitialQuery: "current version",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, correction, review, 2_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SynthesisCalls != 1 || result.SynthesisCorrectionCalls != 1 ||
		result.ClaimEvidenceReviewCalls != 2 || correction.calls != 1 || review.calls != 2 {
		t.Fatalf("result=%#v correction=%d review=%d", result, correction.calls, review.calls)
	}
	if result.Artifact.Paragraphs[0].Text != "Version 2 is current." ||
		fmt.Sprint(result.Artifact.Paragraphs[0].EvidenceIDs) != fmt.Sprint([]EvidenceID{evidence}) ||
		fmt.Sprint(result.Steps) != fmt.Sprint([]Step{
			StepInitialDiscovery, StepDocumentsFetched, StepRelevanceResolved, StepEvidenceProjected,
			StepSynthesisResolved, StepSynthesisCorrected, StepClaimEvidenceReviewed, StepObjectiveCompleted,
		}) {
		t.Fatalf("corrected completion=%#v", result)
	}
	if correction.last.Issue.Detail != "The cited evidence says version 2, not version 3." ||
		len(correction.last.Paragraphs) != 1 || correction.last.Paragraphs[0].Text != "Version 3 is current." ||
		len(correction.last.Evidence) != 1 {
		t.Fatalf("correction lost retained exact input: %#v", correction.last)
	}
}

func TestNoopClaimEvidenceCorrectionRecordsZeroDeltaWithoutAnotherReview(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/already-grounded", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	evidence := evidenceID(document.ID)
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 2 is current.", EvidenceIDs: []EvidenceID{evidence},
	}}}}
	issue := ClaimEvidenceReviewDecision{
		Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1", EvidenceIDs: []EvidenceID{evidence},
		IssueKind: ClaimEvidenceInsufficientSupport, Detail: "The already cited version needs correction.",
	}
	review := &recordingClaimEvidenceReviewStation{decisions: []ClaimEvidenceReviewDecision{issue}}
	correction := &recordingSynthesisCorrectionStation{decision: GroundedSynthesisCorrectionDecision{Text: "Version 2 is current."}}
	machine := newFixtureMachineWithCorrection(t, Objective{
		ID: "objective_zero_delta", Question: "Which version is current?", InitialQuery: "current version",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, correction, review, 2_000)

	result, err := machine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SynthesisCorrectionCalls != 1 ||
		result.SynthesisCorrectionZeroDeltas != 1 || result.ClaimEvidenceReviewCalls != 1 ||
		correction.calls != 1 || review.calls != 1 || result.Artifact.Paragraphs[0].Text != "Version 2 is current." {
		t.Fatalf("zero-delta result=%#v correction=%d review=%d", result, correction.calls, review.calls)
	}
	wantSteps := []Step{
		StepInitialDiscovery, StepDocumentsFetched, StepRelevanceResolved, StepEvidenceProjected,
		StepSynthesisResolved, StepSynthesisZeroDelta, StepClaimEvidenceReviewed, StepObjectiveCompleted,
	}
	if fmt.Sprint(result.Steps) != fmt.Sprint(wantSteps) {
		t.Fatalf("zero-delta steps=%v want %v", result.Steps, wantSteps)
	}
}

func TestSecondClaimEvidenceIssueFailsWithoutAnotherCorrectionOrCompletion(t *testing.T) {
	candidate := candidateFixture("https://evidence.example/repeated-issue", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	evidence := evidenceID(document.ID)
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 3 is current.", EvidenceIDs: []EvidenceID{evidence},
	}}}}
	correction := &recordingSynthesisCorrectionStation{decision: GroundedSynthesisCorrectionDecision{Text: "Version 4 is current."}}
	issue := ClaimEvidenceReviewDecision{
		Outcome: ClaimEvidenceReviewIssue, ParagraphID: "P1", EvidenceIDs: []EvidenceID{evidence},
		IssueKind: ClaimEvidenceContradictedSupport, Detail: "The version claim contradicts the cited evidence.",
	}
	review := &recordingClaimEvidenceReviewStation{decisions: []ClaimEvidenceReviewDecision{issue, issue}}
	machine := newFixtureMachineWithCorrection(t, Objective{
		ID: "objective_repeated_issue", Question: "Which version is current?", InitialQuery: "current version",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, correction, review, 2_000)

	result, err := machine.Run(context.Background())
	if !errors.Is(err, ErrClaimEvidenceInadequate) {
		t.Fatalf("Run error=%v want ErrClaimEvidenceInadequate", err)
	}
	if result.Complete || result.Objective.Status == ObjectiveComplete || result.Artifact.Rendered != "" ||
		result.SynthesisCorrectionCalls != 1 || result.ClaimEvidenceReviewCalls != 2 || correction.calls != 1 {
		t.Fatalf("repeated issue escaped bound: result=%#v correction=%d", result, correction.calls)
	}
}

func TestCancellationAfterClaimEvidenceReviewCannotComplete(t *testing.T) {
	candidate := candidateFixture("https://cancel-review.example/current", "Current")
	document := documentFixture(candidate.URL, candidate.Title, "Version 2 is current.")
	acquisition := &scriptedAcquisition{
		discoveries: map[string]discoverOutcome{"current version": {report: candidateReport("current version", candidate)}},
		documents:   map[websearch.CandidateID]websearch.Document{candidate.ID: document},
	}
	synthesis := &recordingSynthesisStation{decision: GroundedSynthesisDecision{Paragraphs: []GroundedParagraph{{
		Text: "Version 2 is current.", EvidenceIDs: []EvidenceID{evidenceID(document.ID)},
	}}}}
	ctx, cancel := context.WithCancel(context.Background())
	review := &cancelingClaimEvidenceReviewStation{cancel: cancel}
	machine := newFixtureMachineWithReview(t, Objective{
		ID: "objective_review_cancel", Question: "Which version is current?", InitialQuery: "current version",
		Acceptance: exactAcceptance(), Status: ObjectivePending,
	}, acquisition, &recordingTermsStation{}, &recordingRelevanceStation{decision: RelevanceDecision{
		Outcome: RelevanceSelected, CandidateIDs: []websearch.CandidateID{candidate.ID},
	}}, synthesis, review, 2_000)

	result, err := machine.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error=%v want context canceled", err)
	}
	if !reflect.DeepEqual(result, Result{}) || review.calls != 1 {
		t.Fatalf("canceled review completed objective: result=%#v calls=%d", result, review.calls)
	}
}

func TestCanceledContextCannotCommitReviewedArtifact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Result{Objective: Objective{Status: ObjectivePending}}
	err := commitReviewedCompletion(ctx, &result, Artifact{
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

type cancelingClaimEvidenceReviewStation struct {
	cancel context.CancelFunc
	calls  int
}

func (station *cancelingClaimEvidenceReviewStation) Review(
	_ context.Context,
	_ ClaimEvidenceReviewCall,
) (ClaimEvidenceReviewDecision, error) {
	station.calls++
	station.cancel()
	return ClaimEvidenceReviewDecision{Outcome: ClaimEvidenceReviewNone, EvidenceIDs: []EvidenceID{}}, nil
}
