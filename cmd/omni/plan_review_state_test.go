package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestNewPlanReviewStateRequiresReviewAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*model.CodingPlan)
		message string
	}{
		{name: "invalid model authority", mutate: func(plan *model.CodingPlan) {
			plan.Revision = 0
		}, message: "invalid coding plan authority"},
		{name: "nil plan leaves", mutate: func(plan *model.CodingPlan) {
			plan.Leaves = nil
		}, message: "array"},
		{name: "frozen", mutate: freezePlanReviewFixture, message: "not reviewable"},
		{name: "superseded", mutate: func(plan *model.CodingPlan) {
			plan.State = model.CodingPlanStateSuperseded
		}, message: "not reviewable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan := planReviewFixture(t)
			test.mutate(&plan)
			if _, err := newPlanReviewState(plan); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("newPlanReviewState() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestPlanReviewZeroLeafStateOnlyRequestsGuidance(t *testing.T) {
	t.Parallel()

	plan := planReviewFixture(t)
	plan.Leaves = []model.CodingPlanLeaf{}
	state := mustPlanReviewState(t, plan)
	for _, action := range []planReviewAction{
		planReviewActionUp,
		planReviewActionDown,
		planReviewActionToggle,
		planReviewActionEnter,
		planReviewActionCancel,
	} {
		next, effect, err := reducePlanReviewState(state, action)
		if err != nil || effect != (planReviewEffect{}) || next.selected != 0 || next.confirming {
			t.Errorf("zero-leaf action %q = state %#v, effect %#v, error %v", action, next, effect, err)
		}
	}
	next, effect, err := reducePlanReviewState(state, planReviewActionRequestNote)
	if err != nil || effect.Kind != planReviewEffectNoteRequested || effect.LeafID != "" ||
		next.selected != 0 || next.confirming {
		t.Fatalf("zero-leaf note = state %#v, effect %#v, error %v", next, effect, err)
	}
}

func TestPlanReviewStateClonesAuthoritativeLeaves(t *testing.T) {
	t.Parallel()

	plan := planReviewFixture(t)
	state := mustPlanReviewState(t, plan)
	plan.Leaves[0].Statement = "mutated by caller"
	if state.snapshot.Leaves[0].Statement != "Create the grounded behavior" {
		t.Fatalf("retained statement = %q", state.snapshot.Leaves[0].Statement)
	}
}

func TestPlanReviewNavigationIsBounded(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	state = reducePlanReview(t, state, planReviewActionUp)
	if state.selected != 0 {
		t.Fatalf("selection after bounded Up = %d, want 0", state.selected)
	}
	for range len(state.snapshot.Leaves) + 2 {
		state = reducePlanReview(t, state, planReviewActionDown)
	}
	if state.selected != len(state.snapshot.Leaves)-1 {
		t.Fatalf("selection after bounded Down = %d", state.selected)
	}
	state = reducePlanReview(t, state, planReviewActionUp)
	if state.selected != len(state.snapshot.Leaves)-2 {
		t.Fatalf("selection after Up = %d", state.selected)
	}
}

func TestPlanReviewToggleRequestsServerDecisionWithoutOptimisticMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		current model.CodingPlanDecision
		desired model.CodingPlanDecision
	}{
		{current: model.CodingPlanDecisionPending, desired: model.CodingPlanDecisionApproved},
		{current: model.CodingPlanDecisionRejected, desired: model.CodingPlanDecisionApproved},
		{current: model.CodingPlanDecisionApproved, desired: model.CodingPlanDecisionRejected},
	}
	for _, test := range tests {
		t.Run(string(test.current), func(t *testing.T) {
			t.Parallel()
			plan := planReviewFixture(t)
			plan.Leaves[0].Decision = test.current
			state := mustPlanReviewState(t, plan)
			next, effect, err := reducePlanReviewState(state, planReviewActionToggle)
			if err != nil {
				t.Fatal(err)
			}
			want := planReviewEffect{
				Kind:   planReviewEffectDecisionRequested,
				LeafID: state.snapshot.Leaves[0].ID, Decision: test.desired,
			}
			if effect != want {
				t.Fatalf("toggle effect = %#v, want %#v", effect, want)
			}
			if next.snapshot.Leaves[0].Decision != test.current {
				t.Fatalf("toggle changed authoritative decision to %q", next.snapshot.Leaves[0].Decision)
			}
		})
	}
}

func TestPlanReviewToggleCannotAuthorizeFreezeBeforeServerPersistence(t *testing.T) {
	t.Parallel()

	plan := eligiblePlanReviewFixture(t)
	plan.Leaves[0].Decision = model.CodingPlanDecisionPending
	state := mustPlanReviewState(t, plan)
	next, effect, err := reducePlanReviewState(state, planReviewActionToggle)
	if err != nil || effect.Kind != planReviewEffectDecisionRequested {
		t.Fatalf("toggle = effect %#v, error %v", effect, err)
	}
	next, effect, err = reducePlanReviewState(next, planReviewActionEnter)
	if err != nil || effect != (planReviewEffect{}) || next.confirming {
		t.Fatalf(
			"Enter before persisted response = confirming %t, effect %#v, error %v",
			next.confirming,
			effect,
			err,
		)
	}
}

func TestPlanReviewConflictAnnotationRemainsUserDecidable(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	state.selected = 3
	next, effect, err := reducePlanReviewState(state, planReviewActionToggle)
	if err != nil {
		t.Fatal(err)
	}
	want := planReviewEffect{
		Kind: planReviewEffectDecisionRequested,
		LeafID: state.snapshot.Leaves[3].ID, Decision: model.CodingPlanDecisionApproved,
	}
	if effect != want || next.snapshot.Leaves[3].Decision != model.CodingPlanDecisionRejected {
		t.Fatalf("conflict toggle = state %#v, effect %#v", next, effect)
	}
}

func TestPlanReviewConfirmationRequiresPersistedCompleteApprovalSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		decisions []model.CodingPlanDecision
		wantOpen  bool
	}{
		{
			name: "pending remains",
			decisions: []model.CodingPlanDecision{
				model.CodingPlanDecisionApproved,
				model.CodingPlanDecisionPending,
				model.CodingPlanDecisionRejected,
			},
		},
		{
			name: "nothing approved",
			decisions: []model.CodingPlanDecision{
				model.CodingPlanDecisionRejected,
				model.CodingPlanDecisionRejected,
				model.CodingPlanDecisionRejected,
			},
		},
		{
			name: "complete accepted set",
			decisions: []model.CodingPlanDecision{
				model.CodingPlanDecisionApproved,
				model.CodingPlanDecisionRejected,
				model.CodingPlanDecisionRejected,
			},
			wantOpen: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan := planReviewFixture(t)
			for index, decision := range test.decisions {
				plan.Leaves[index].Decision = decision
			}
			state := mustPlanReviewState(t, plan)
			next, effect, err := reducePlanReviewState(state, planReviewActionEnter)
			if err != nil {
				t.Fatal(err)
			}
			if effect != (planReviewEffect{}) || next.confirming != test.wantOpen {
				t.Fatalf("Enter = confirming %t, effect %#v; want %t", next.confirming, effect, test.wantOpen)
			}
		})
	}
}

func TestPlanReviewConfirmationNeedsSecondEnterAndCanBeCanceled(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state = reducePlanReview(t, state, planReviewActionEnter)
	if !state.confirming {
		t.Fatal("first Enter did not open confirmation")
	}
	next, effect, err := reducePlanReviewState(state, planReviewActionEnter)
	if err != nil {
		t.Fatal(err)
	}
	if !next.confirming || effect.Kind != planReviewEffectFreezeRequested {
		t.Fatalf("second Enter = confirming %t, effect %#v", next.confirming, effect)
	}

	canceled := reducePlanReview(t, state, planReviewActionCancel)
	if canceled.confirming {
		t.Fatal("Cancel did not exit confirmation")
	}
	next, effect, err = reducePlanReviewState(canceled, planReviewActionEnter)
	if err != nil || effect != (planReviewEffect{}) || !next.confirming {
		t.Fatalf("Enter after cancel = confirming %t, effect %#v, error %v", next.confirming, effect, err)
	}
}

func TestPlanReviewNoteSignalsSelectedLeafAndExitsConfirmation(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state.selected = 1
	state.confirming = true
	next, effect, err := reducePlanReviewState(state, planReviewActionRequestNote)
	if err != nil {
		t.Fatal(err)
	}
	want := planReviewEffect{
		Kind: planReviewEffectNoteRequested, LeafID: state.snapshot.Leaves[1].ID,
	}
	if effect != want || next.confirming {
		t.Fatalf("note = state %#v, effect %#v; want %#v", next, effect, want)
	}
}

func TestPlanReviewNoteSignalsSelectedLeafDuringOrdinaryReview(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	state.selected = 2
	next, effect, err := reducePlanReviewState(state, planReviewActionRequestNote)
	if err != nil {
		t.Fatal(err)
	}
	want := planReviewEffect{
		Kind: planReviewEffectNoteRequested, LeafID: state.snapshot.Leaves[2].ID,
	}
	if effect != want || next.selected != state.selected || next.confirming {
		t.Fatalf("ordinary note = state %#v, effect %#v; want %#v", next, effect, want)
	}
}

func TestPlanReviewConfirmationIgnoresSelectionAndToggleKeys(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state.confirming = true
	for _, action := range []planReviewAction{
		planReviewActionUp, planReviewActionDown, planReviewActionToggle,
	} {
		next, effect, err := reducePlanReviewState(state, action)
		if err != nil || effect != (planReviewEffect{}) || next.selected != state.selected || !next.confirming {
			t.Errorf("confirmation action %q = state %#v, effect %#v, error %v", action, next, effect, err)
		}
	}
}

func TestPlanReviewRejectsUnknownOrCorruptInput(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, planReviewFixture(t))
	if _, _, err := reducePlanReviewState(state, "left"); err == nil {
		t.Fatal("unknown action was accepted")
	}
	state.selected = len(state.snapshot.Leaves)
	if _, _, err := reducePlanReviewState(state, planReviewActionDown); err == nil {
		t.Fatal("out-of-range retained selection was accepted")
	}
}
