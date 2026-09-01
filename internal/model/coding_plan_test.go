package model

import (
	"strings"
	"testing"
	"time"
)

func TestCodingPlanLeafIdentityBindsExactStatement(t *testing.T) {
	id, err := NewCodingPlanLeafID("The software lets a user confirm the item.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), "coding_plan_leaf_") {
		t.Fatalf("leaf id=%q", id)
	}
	other, err := NewCodingPlanLeafID("The software lets a user confirm the item")
	if err != nil {
		t.Fatal(err)
	}
	if id == other {
		t.Fatal("different exact statements shared one leaf identity")
	}
}

func TestFrozenCodingPlanRequiresDecisionsAndApprovedWork(t *testing.T) {
	now := time.Now().UTC()
	statement := "The software lets a user confirm the item."
	id, err := NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	plan := CodingPlan{
		JobID: 1, Generation: 1, Revision: 2, State: CodingPlanStateFrozen,
		ScopeMode:     CodingScopeModeNormal,
		RequestSHA256: strings.Repeat("a", 64),
		Leaves: []CodingPlanLeaf{{
			ID: id, Statement: statement, Annotation: CodingPlanAnnotationGrounded,
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

func TestConcreteConflictAnnotationDoesNotOwnTheUserDecision(t *testing.T) {
	statement := "Add an unrelated export service."
	id, err := NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	leaf := CodingPlanLeaf{
		ID: id, Statement: statement,
		Annotation: CodingPlanAnnotationConcreteConflict,
		Decision:   CodingPlanDecisionApproved,
	}
	if err := leaf.Validate(); err != nil {
		t.Fatalf("approved annotated conflict: %v", err)
	}
	leaf.Decision = CodingPlanDecisionRejected
	if err := leaf.Validate(); err != nil {
		t.Fatalf("rejected annotated conflict: %v", err)
	}
}
