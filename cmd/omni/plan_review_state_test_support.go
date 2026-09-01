package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func planReviewFixture(t *testing.T) model.CodingPlan {
	t.Helper()
	created := time.Unix(1_700_000_000, 0).UTC()
	return model.CodingPlan{
		JobID: 42, Generation: 3, Revision: 1,
		State: model.CodingPlanStateReview, ScopeMode: model.CodingScopeModeNormal,
		RequestSHA256: strings.Repeat("a", 64),
		Leaves: []model.CodingPlanLeaf{
			planReviewFixtureLeaf(
				t,
				"Create the grounded behavior",
				model.CodingPlanAnnotationGrounded,
				model.CodingPlanDecisionApproved,
			),
			planReviewFixtureLeaf(
				t,
				"Include the reasonable derivation",
				model.CodingPlanAnnotationReasonableDerivation,
				model.CodingPlanDecisionPending,
			),
			planReviewFixtureLeaf(
				t,
				"Consider the speculative detail",
				model.CodingPlanAnnotationSpeculativeReview,
				model.CodingPlanDecisionRejected,
			),
			planReviewFixtureLeaf(
				t,
				"Do the conflicting thing",
				model.CodingPlanAnnotationConcreteConflict,
				model.CodingPlanDecisionRejected,
			),
		},
		CreatedAt: created,
		UpdatedAt: created.Add(time.Second),
	}
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
	annotation model.CodingPlanAnnotation,
	decision model.CodingPlanDecision,
) model.CodingPlanLeaf {
	t.Helper()
	id, err := model.NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	return model.CodingPlanLeaf{
		ID: id, Statement: statement, Annotation: annotation, Decision: decision,
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
