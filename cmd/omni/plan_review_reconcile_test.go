package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanReviewReconciliationUsesServerRevisionAndStableCursor(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state.selected = 1
	state.confirming = true

	unchangedPlan := eligiblePlanReviewFixture(t)
	unchanged, change, err := reconcilePlanReviewState(state, unchangedPlan)
	if err != nil || change != planReviewSnapshotUnchanged ||
		unchanged.selected != 1 || !unchanged.confirming {
		t.Fatalf("unchanged reconciliation = %#v, %q, %v", unchanged, change, err)
	}

	revised := eligiblePlanReviewFixture(t)
	revised.Revision = 2
	revised.UpdatedAt = revised.UpdatedAt.Add(time.Second)
	revised.Leaves[1].Decision = model.CodingPlanDecisionPending
	next, change, err := reconcilePlanReviewState(state, revised)
	if err != nil {
		t.Fatal(err)
	}
	if change != planReviewSnapshotRevisionChanged || next.selected != 1 || next.confirming {
		t.Fatalf("revised reconciliation = selected %d, confirming %t, change %q", next.selected, next.confirming, change)
	}
	if next.snapshot.Leaves[next.selected].ID != state.snapshot.Leaves[1].ID ||
		next.snapshot.Leaves[next.selected].Decision != model.CodingPlanDecisionPending {
		t.Fatalf("server revision was not authoritative: %#v", next.snapshot.Leaves[next.selected])
	}

	replanned := revised
	replanned.Generation++
	replanned.Revision = 1
	replanned.UpdatedAt = replanned.UpdatedAt.Add(time.Second)
	replanned.Leaves[0], replanned.Leaves[1] = replanned.Leaves[1], replanned.Leaves[0]
	next, change, err = reconcilePlanReviewState(next, replanned)
	if err != nil || change != planReviewSnapshotIdentityChanged || next.selected != 0 {
		t.Fatalf("generation reconciliation = selected %d, change %q, error %v", next.selected, change, err)
	}

	otherJob := replanned
	otherJob.JobID++
	otherJob.Generation = 1
	next, change, err = reconcilePlanReviewState(next, otherJob)
	if err != nil || change != planReviewSnapshotIdentityChanged || next.selected != 0 {
		t.Fatalf("job reconciliation = selected %d, change %q, error %v", next.selected, change, err)
	}
}

func TestPlanReviewReconciliationResetsMissingSelection(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	state.selected = 2
	revised := eligiblePlanReviewFixture(t)
	revised.Generation++
	revised.Revision = 1
	revised.UpdatedAt = revised.UpdatedAt.Add(time.Second)
	revised.Leaves = slices.Delete(revised.Leaves, 2, 3)
	next, _, err := reconcilePlanReviewState(state, revised)
	if err != nil {
		t.Fatal(err)
	}
	if next.selected != 0 || next.snapshot.Leaves[0].ID != state.snapshot.Leaves[0].ID {
		t.Fatalf("missing selection reconciled to index %d", next.selected)
	}
}

func TestPlanReviewReconciliationHandlesZeroLeafGenerations(t *testing.T) {
	t.Parallel()

	empty := planReviewFixture(t)
	empty.Leaves = []model.CodingPlanLeaf{}
	state := mustPlanReviewState(t, empty)
	unchanged, change, err := reconcilePlanReviewState(state, empty)
	if err != nil || change != planReviewSnapshotUnchanged || unchanged.selected != 0 {
		t.Fatalf("empty unchanged reconciliation = state %#v, change %q, error %v", unchanged, change, err)
	}

	withLeaves := planReviewFixture(t)
	withLeaves.Generation++
	withLeaves.Revision = 1
	withLeaves.UpdatedAt = withLeaves.UpdatedAt.Add(time.Second)
	next, change, err := reconcilePlanReviewState(state, withLeaves)
	if err != nil || change != planReviewSnapshotIdentityChanged || next.selected != 0 ||
		len(next.snapshot.Leaves) == 0 {
		t.Fatalf("empty-to-leaves reconciliation = state %#v, change %q, error %v", next, change, err)
	}

	nextEmpty := withLeaves
	nextEmpty.Generation++
	nextEmpty.Revision = 1
	nextEmpty.UpdatedAt = nextEmpty.UpdatedAt.Add(time.Second)
	nextEmpty.Leaves = []model.CodingPlanLeaf{}
	next, change, err = reconcilePlanReviewState(next, nextEmpty)
	if err != nil || change != planReviewSnapshotIdentityChanged || next.selected != 0 ||
		len(next.snapshot.Leaves) != 0 {
		t.Fatalf("leaves-to-empty reconciliation = state %#v, change %q, error %v", next, change, err)
	}
}

func TestPlanReviewReconciliationRejectsContradictoryOrStaleAuthority(t *testing.T) {
	t.Parallel()

	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	sameRevision := eligiblePlanReviewFixture(t)
	sameRevision.Leaves[0].Decision = model.CodingPlanDecisionRejected
	if _, _, err := reconcilePlanReviewState(state, sameRevision); err == nil ||
		!strings.Contains(err.Error(), "without a new revision") {
		t.Fatalf("same-revision mutation error = %v", err)
	}

	newer := eligiblePlanReviewFixture(t)
	newer.Revision = 3
	state = mustPlanReviewState(t, newer)
	stale := eligiblePlanReviewFixture(t)
	stale.Revision = 2
	if _, _, err := reconcilePlanReviewState(state, stale); err == nil ||
		!strings.Contains(err.Error(), "regressed") {
		t.Fatalf("revision regression error = %v", err)
	}
}

func TestPlanReviewRevisionCannotRewriteImmutableAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*model.CodingPlan)
		message string
	}{
		{
			name: "request",
			mutate: func(plan *model.CodingPlan) {
				plan.RequestSHA256 = strings.Repeat("b", 64)
			},
			message: "request authority",
		},
		{
			name: "scope",
			mutate: func(plan *model.CodingPlan) {
				plan.ScopeMode = model.CodingScopeModeStrict
			},
			message: "scope mode",
		},
		{
			name: "creation time",
			mutate: func(plan *model.CodingPlan) {
				plan.CreatedAt = plan.CreatedAt.Add(time.Second)
				plan.UpdatedAt = plan.CreatedAt
			},
			message: "creation authority",
		},
		{
			name: "update time regression",
			mutate: func(plan *model.CodingPlan) {
				plan.UpdatedAt = plan.CreatedAt
			},
			message: "update time regressed",
		},
		{
			name: "leaf order",
			mutate: func(plan *model.CodingPlan) {
				plan.Leaves[0], plan.Leaves[1] = plan.Leaves[1], plan.Leaves[0]
			},
			message: "leaf 0 authority",
		},
		{
			name: "leaf annotation",
			mutate: func(plan *model.CodingPlan) {
				plan.Leaves[0].Annotation = model.CodingPlanAnnotationSpeculativeReview
			},
			message: "leaf 0 authority",
		},
		{
			name: "leaf set",
			mutate: func(plan *model.CodingPlan) {
				plan.Leaves = plan.Leaves[:len(plan.Leaves)-1]
			},
			message: "leaf set",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
			revised := eligiblePlanReviewFixture(t)
			revised.Revision++
			revised.UpdatedAt = revised.UpdatedAt.Add(2 * time.Second)
			test.mutate(&revised)
			if _, _, err := reconcilePlanReviewState(state, revised); err == nil ||
				!strings.Contains(err.Error(), test.message) {
				t.Fatalf("immutable %s error = %v", test.name, err)
			}
		})
	}
}
