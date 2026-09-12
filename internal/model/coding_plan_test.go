package model

import (
	"strings"
	"testing"
	"time"
)

func TestCodingPlanLeafIdentityIsNotAStatementHash(t *testing.T) {
	id, err := NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), "coding_plan_leaf_") {
		t.Fatalf("leaf id=%q", id)
	}
	other, err := NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	if id == other {
		t.Fatal("two new leaves shared one identity")
	}
	if _, err := ParseCodingPlanLeafID("coding_plan_leaf_" + strings.Repeat("a", 64)); err == nil {
		t.Fatal("accepted an obsolete statement-hash identity")
	}
}

func TestFrozenCodingPlanRequiresDecisionsAndApprovedWork(t *testing.T) {
	now := time.Now().UTC()
	statement := "The software lets a user confirm the item."
	id, err := NewCodingPlanLeafID()
	if err != nil {
		t.Fatal(err)
	}
	plan := CodingPlan{
		JobID: 1, Generation: 1, Revision: 2, State: CodingPlanStateFrozen,
		Leaves: []CodingPlanLeaf{{
			ID: id, Statement: statement,
			Decision: CodingPlanDecisionPending,
		}},
		CreatedAt: now, UpdatedAt: now, FrozenAt: &now,
	}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("pending frozen plan error=%v", err)
	}
	plan.Leaves[0].Decision = CodingPlanDecisionRejected
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("all-rejected frozen plan error=%v", err)
	}
	plan.Leaves[0].Decision = CodingPlanDecisionApproved
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
}
