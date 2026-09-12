package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func planReviewFixture(t *testing.T) model.CodingPlan {
	t.Helper()
	created := time.Unix(1_700_000_000, 0).UTC()
	plan := model.CodingPlan{
		JobID: 42, Generation: 3, Revision: 1,
		State: model.CodingPlanStateReview,
		Leaves: []model.CodingPlanLeaf{
			planReviewFixtureLeaf(
				t,
				"Confirm the selected item",
				model.CodingPlanDecisionApproved,
			),
			planReviewFixtureLeaf(
				t,
				"Display the item's status",
				model.CodingPlanDecisionPending,
			),
			planReviewFixtureLeaf(
				t,
				"Archive the selected item",
				model.CodingPlanDecisionRejected,
			),
			planReviewFixtureLeaf(
				t,
				"Restore an archived item",
				model.CodingPlanDecisionRejected,
			),
		},
		CreatedAt: created,
		UpdatedAt: created.Add(time.Second),
	}
	for index := range plan.Leaves {
		plan.Leaves[index].ID = model.CodingPlanLeafID(fmt.Sprintf("coding_plan_leaf_%032x", index+1))
	}
	return plan
}

func eligiblePlanReviewFixture(t *testing.T) model.CodingPlan {
	t.Helper()
	plan := planReviewFixture(t)
	plan.Leaves[1].Decision = model.CodingPlanDecisionRejected
	return plan
}

func freezePlanReviewFixture(plan *model.CodingPlan) {
	plan.State = model.CodingPlanStateFrozen
	for index := range plan.Leaves {
		if plan.Leaves[index].Decision == model.CodingPlanDecisionPending {
			plan.Leaves[index].Decision = model.CodingPlanDecisionRejected
		}
	}
	frozenAt := plan.UpdatedAt.Add(time.Second)
	plan.UpdatedAt = frozenAt
	plan.FrozenAt = &frozenAt
}

func planReviewFixtureLeaf(
	t *testing.T,
	statement string,
	decision model.CodingPlanDecision,
) model.CodingPlanLeaf {
	t.Helper()
	id, err := model.NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	return model.CodingPlanLeaf{
		ID: id, Statement: statement, Decision: decision,
	}
}

func mustPlanReviewState(t *testing.T, plan model.CodingPlan) planReviewState {
	t.Helper()
	state, err := newPlanReviewState(plan)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func reducePlanReview(t *testing.T, state planReviewState, action planReviewAction) planReviewState {
	t.Helper()
	next, effect, err := reducePlanReviewState(state, action)
	if err != nil {
		t.Fatal(err)
	}
	if effect != (planReviewEffect{}) {
		t.Fatalf("unexpected plan review effect %#v", effect)
	}
	return next
}
