package semanticreview

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

type semanticReviewFixture struct {
	name        string
	objective   Objective
	artifact    string
	requirement string
	issue       FindingCode
	kind        CorrectionObjectiveKind
	choices     []string
	structural  func(string) bool
	accept      func(string) bool
	correct     func(string) string
}

func semanticReviewFixtures() []semanticReviewFixture {
	return []semanticReviewFixture{
		{
			name:        "retry explanation",
			objective:   Objective{ID: "O_retry", Acceptance: requiredRootAcceptance(), Status: ObjectivePending},
			artifact:    "Failures are retried immediately.",
			requirement: "Retries must wait before another attempt.",
			issue:       "F_waiting_semantics", kind: "C_rewrite_explanation",
			choices:    []string{"C17", "C99"},
			structural: func(value string) bool { return strings.TrimSpace(value) != "" },
			accept:     func(value string) bool { return value == "Failures are retried after a delay." },
			correct:    func(string) string { return "Failures are retried after a delay." },
		},
		{
			name:        "simulation appraisal",
			objective:   Objective{ID: "O_appraisal", Acceptance: requiredRootAcceptance(), Status: ObjectivePending},
			artifact:    "The danger is still active.",
			requirement: "The observed danger has cleared.",
			issue:       "F_cleared_state", kind: "C_reconcile_appraisal",
			choices:    []string{"C17", "C99"},
			structural: func(value string) bool { return strings.TrimSpace(value) != "" },
			accept:     func(value string) bool { return value == "The danger has passed." },
			correct:    func(string) string { return "The danger has passed." },
		},
	}
}

func newFixtureMachine(
	t *testing.T,
	fixture semanticReviewFixture,
	selector cognitionreference.Selector,
	verifier Verifier,
	executor CorrectionExecutor,
	maxRounds int,
) Machine {
	t.Helper()
	initial, err := NewInitialArtifact(fixture.objective.ID, []byte(fixture.artifact))
	if err != nil {
		t.Fatal(err)
	}
	specification := ReviewSpecification{
		ID:          "RS_" + ReviewSpecificationID(fixture.objective.ID),
		ObjectiveID: fixture.objective.ID,
		Question:    "Does the current bounded result contradict the exact requirement?",
		Evidence: []EvidenceDefinition{
			{ID: "E01", Kind: EvidenceFixed, Content: fixture.requirement},
			{ID: "E02", Kind: EvidenceCurrentArtifact},
		},
		Candidates: []FindingDefinition{
			{CandidateID: "C17", FindingCode: fixture.issue, Kind: FindingSemanticIssue, Summary: "The current result contradicts the requirement.", EvidenceIDs: []cognitionreference.EvidenceID{"E01", "E02"}},
			{CandidateID: "C99", FindingCode: FindingCodeNone, Kind: FindingNone, Summary: "No semantic contradiction is present.", EvidenceIDs: []cognitionreference.EvidenceID{"E01", "E02"}},
		},
	}
	rules, err := NewCorrectionRuleRegistry(specification, []CorrectionRule{{
		FindingCode: fixture.issue, ObjectiveKind: fixture.kind,
		Acceptance: []CorrectionAcceptancePredicate{AcceptanceCorrectionArtifactVerified},
	}})
	if err != nil {
		t.Fatal(err)
	}
	executors, err := NewCorrectionExecutorRegistry(rules, []CorrectionExecutorRegistration{{
		ObjectiveKind: fixture.kind, Executor: executor,
	}})
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewMachine(
		fixture.objective, initial, specification, rules, executors,
		selector, verifier, Limits{MaxReviewRounds: maxRounds},
	)
	if err != nil {
		t.Fatal(err)
	}
	return machine
}

type scriptedSelector struct {
	choices  []string
	calls    int
	gaps     []cognitionreference.SemanticGap
	err      error
	cancel   context.CancelFunc
	cancelAt int
	mutate   bool
}

func (selector *scriptedSelector) Select(
	_ context.Context,
	gap cognitionreference.SemanticGap,
) (cognitionreference.CandidateID, error) {
	selector.calls++
	selector.gaps = append(selector.gaps, gap.Clone())
	if selector.mutate {
		gap.Question = "mutated"
		gap.Candidates[0].Summary = "mutated"
		gap.Evidence[0].Content = "mutated"
	}
	if selector.cancel != nil && (selector.cancelAt == 0 || selector.cancelAt == selector.calls) {
		selector.cancel()
	}
	if selector.err != nil {
		return "", selector.err
	}
	if selector.calls > len(selector.choices) {
		return "", fmt.Errorf("unexpected selection call %d", selector.calls)
	}
	return cognitionreference.CandidateID(selector.choices[selector.calls-1]), nil
}

type scriptedVerifier struct {
	structural       func(string) bool
	acceptCorrection func(string) bool
	calls            int
	err              error
	errAt            int
	cancel           context.CancelFunc
	mutate           bool
	inputs           []VerificationInput
	cancelAt         int
}

func (verifier *scriptedVerifier) Verify(_ context.Context, input VerificationInput) error {
	verifier.calls++
	verifier.inputs = append(verifier.inputs, cloneVerificationInput(input))
	value := string(input.Artifact.Content)
	if verifier.mutate && len(input.Artifact.Content) > 0 {
		input.Artifact.Content[0] ^= 0xff
	}
	if verifier.cancel != nil && (verifier.cancelAt == 0 || verifier.cancelAt == verifier.calls) {
		verifier.cancel()
	}
	if verifier.err != nil && (verifier.errAt == 0 || verifier.errAt == verifier.calls) {
		return verifier.err
	}
	switch input.Kind {
	case VerificationCurrentArtifact:
		if verifier.structural != nil && !verifier.structural(value) {
			return fmt.Errorf("current artifact is not structurally acceptable")
		}
	case VerificationCorrectionArtifact:
		if verifier.acceptCorrection != nil && !verifier.acceptCorrection(value) {
			return fmt.Errorf("correction artifact does not satisfy its registered acceptance")
		}
	default:
		return fmt.Errorf("unknown verification kind %q", input.Kind)
	}
	return nil
}

type scriptedCorrectionExecutor struct {
	correct func(string) string
	calls   int
	err     error
	cancel  context.CancelFunc
	mutate  bool
}

func (executor *scriptedCorrectionExecutor) Execute(
	_ context.Context,
	objective CorrectionObjective,
	artifact Artifact,
) (ArtifactValue, error) {
	executor.calls++
	if executor.mutate {
		objective.Acceptance[0] = "mutated"
		objective.Finding.EvidenceIDs[0] = "mutated"
		artifact.Content[0] ^= 0xff
	}
	if executor.cancel != nil {
		executor.cancel()
	}
	if executor.err != nil {
		return ArtifactValue{}, executor.err
	}
	if executor.correct == nil {
		return ArtifactValue{Content: bytes.Clone(artifact.Content)}, nil
	}
	return ArtifactValue{Content: []byte(executor.correct(string(artifact.Content)))}, nil
}

func requiredRootAcceptance() []AcceptancePredicate {
	return []AcceptancePredicate{
		AcceptanceCurrentArtifactVerified,
		AcceptanceNoOpenSemanticFinding,
	}
}

func gapContains(gap cognitionreference.SemanticGap, value string) bool {
	for _, evidence := range gap.Evidence {
		if strings.Contains(evidence.Content, value) {
			return true
		}
	}
	return false
}

func assertGapCarriesOnlyCurrentArtifact(t *testing.T, gap cognitionreference.SemanticGap, artifact Artifact) {
	t.Helper()
	count := 0
	for _, evidence := range gap.Evidence {
		if bytes.Equal([]byte(evidence.Content), artifact.Content) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("current artifact occurrences=%d in gap %+v", count, gap)
	}
}
