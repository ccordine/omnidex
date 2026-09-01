package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestRenderPlanReviewShowsLeavesLegendDecisionsAndGuidance(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	rendered, err := renderPlanReview(state)
	if err != nil {
		t.Fatal(err)
	}
	want := "PLAN REVIEW\n" +
		"job 42 · generation 3 · state review · scope normal · revision 1\n\n" +
		"> 1. [approved] ✓ Create the grounded behavior\n" +
		"  2. [pending] ~ Include the reasonable derivation\n" +
		"  3. [rejected] ? Consider the speculative detail\n" +
		"  4. [rejected] ! Do the conflicting thing\n\n" +
		"Annotations: ✓ grounded · ~ reasonable derivation · ? speculative review · ! concrete scope conflict\n" +
		"Controls: ↑/↓ move · Space approve/reject · N note and replan this same job · Enter open freeze confirmation · Ctrl-D exit client\n" +
		"Freeze unavailable: decide 1 pending leaf.\n"
	if rendered != want {
		t.Fatalf("renderPlanReview() =\n%s\nwant:\n%s", rendered, want)
	}
	for _, leaf := range state.snapshot.Leaves {
		if strings.Contains(rendered, string(leaf.ID)) {
			t.Errorf("render exposed internal stable leaf ID %q", leaf.ID)
		}
	}
}

func TestRenderPlanReviewZeroLeafStateOffersOnlyGuidanceAndExit(t *testing.T) {
	t.Parallel()

	plan := planReviewFixture(t)
	plan.Leaves = []model.CodingPlanLeaf{}
	rendered, err := renderPlanReview(mustPlanReviewState(t, plan))
	if err != nil {
		t.Fatal(err)
	}
	want := "PLAN REVIEW\n" +
		"job 42 · generation 3 · state review · scope normal · revision 1\n\n" +
		"No proposed coding leaves were produced for this objective.\n\n" +
		"Guidance is required before coding can start.\n" +
		"Controls: N note and replan this same job · Ctrl-D exit client\n"
	if rendered != want {
		t.Fatalf("renderPlanReview() =\n%s\nwant:\n%s", rendered, want)
	}
	for _, forbidden := range []string{"Space", "Enter", "approve", "freeze"} {
		if strings.Contains(strings.ToLower(rendered), strings.ToLower(forbidden)) {
			t.Errorf("zero-leaf guidance unexpectedly offers %q:\n%s", forbidden, rendered)
		}
	}
}

func TestRenderPlanReviewConfirmationRequiresAnotherEnter(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state.confirming = true
	rendered, err := renderPlanReview(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		"Freeze this work set: 1 approved · 3 rejected.",
		"Press Enter again to confirm and start coding · Esc cancel · N note and replan this same job",
	} {
		if !strings.Contains(rendered, exact) {
			t.Errorf("confirmation render lacks %q:\n%s", exact, rendered)
		}
	}
	if strings.Contains(rendered, "Controls: ↑/↓") {
		t.Fatalf("confirmation render retained ordinary controls:\n%s", rendered)
	}
}

func TestRenderPlanReviewExplainsEachFreezeEligibilityState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*model.CodingPlan)
		expected string
	}{
		{
			name: "pending and no approval",
			mutate: func(plan *model.CodingPlan) {
				plan.Leaves[0].Decision = model.CodingPlanDecisionRejected
			},
			expected: "decide 1 pending leaf and approve at least one leaf",
		},
		{
			name: "decided without approval",
			mutate: func(plan *model.CodingPlan) {
				for index := range plan.Leaves {
					plan.Leaves[index].Decision = model.CodingPlanDecisionRejected
				}
			},
			expected: "Freeze unavailable: approve at least one leaf.",
		},
		{
			name: "eligible",
			mutate: func(plan *model.CodingPlan) {
				plan.Leaves[1].Decision = model.CodingPlanDecisionRejected
			},
			expected: "All leaves are decided. Press Enter to inspect the frozen work set.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan := planReviewFixture(t)
			test.mutate(&plan)
			rendered, err := renderPlanReview(mustPlanReviewState(t, plan))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rendered, test.expected) {
				t.Fatalf("render lacks %q:\n%s", test.expected, rendered)
			}
		})
	}
}

func TestRenderPlanReviewEscapesTerminalControlText(t *testing.T) {
	t.Parallel()

	plan := planReviewFixture(t)
	plan.Leaves[0] = planReviewFixtureLeaf(
		t,
		"first line\nsecond\t\x1b[2J\u009b31m\u202ereversed",
		model.CodingPlanAnnotationGrounded,
		model.CodingPlanDecisionApproved,
	)
	state := mustPlanReviewState(t, plan)
	rendered, err := renderPlanReview(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(rendered, '\x1b') || strings.ContainsRune(rendered, '\u009b') ||
		strings.ContainsRune(rendered, '\u202e') || strings.Contains(rendered, "first line\nsecond") {
		t.Fatalf("render contains raw terminal control text: %q", rendered)
	}
	if escaped := `first line\nsecond\t\x1b[2J\u009b31m\u202ereversed`; !strings.Contains(rendered, escaped) {
		t.Errorf("safe render lacks %q: %q", escaped, rendered)
	}
}

func TestPlanReviewAnnotationPresentationIsComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		annotation model.CodingPlanAnnotation
		symbol     string
		label      string
	}{
		{model.CodingPlanAnnotationGrounded, "✓", "grounded"},
		{model.CodingPlanAnnotationReasonableDerivation, "~", "reasonable derivation"},
		{model.CodingPlanAnnotationSpeculativeReview, "?", "speculative review"},
		{model.CodingPlanAnnotationConcreteConflict, "!", "concrete scope conflict"},
	}
	for _, test := range tests {
		symbol, label := planReviewAnnotationPresentation(test.annotation)
		if symbol != test.symbol || label != test.label {
			t.Errorf("presentation(%q) = (%q, %q)", test.annotation, symbol, label)
		}
	}
}

func TestRenderPlanReviewRejectsCorruptRetainedState(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	state.selected = -1
	if _, err := renderPlanReview(state); err == nil {
		t.Fatal("render accepted an invalid retained selection")
	}
}
