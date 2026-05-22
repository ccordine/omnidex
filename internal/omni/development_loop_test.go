package omni

import (
	"strings"
	"testing"
)

func TestValidateStructuredProofPlanAcceptsUserExplicitObjective(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:          "create_notes_crud",
		Description: "Create notes CRUD",
		Status:      "pending",
		Source:      structuredObjectiveSourceUserExplicit,
		Required:    true,
	}}
	plan := StructuredProofPlan{
		ObjectiveID:      "create_notes_crud",
		ProofType:        structuredProofTypeSmokeTest,
		FilesToCreate:    []string{"src/App.test.jsx"},
		Commands:         []string{"npm test -- --run"},
		AcceptanceChecks: []string{"user can create a note", "user can delete a note"},
		OutOfScope:       []string{"authentication", "cloud sync", "routing"},
	}
	if err := validateStructuredProofPlan(plan, ledger); err != nil {
		t.Fatalf("expected proof plan to validate: %v", err)
	}
}

func TestValidateStructuredProofPlanRejectsMemorySuggestedObjective(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:          "add_tailwind",
		Description: "Use preferred Tailwind stack",
		Status:      "pending",
		Source:      structuredObjectiveSourceMemorySuggested,
		Required:    false,
	}}
	plan := StructuredProofPlan{
		ObjectiveID:      "add_tailwind",
		ProofType:        structuredProofTypeSmokeTest,
		Commands:         []string{"npm test -- --run"},
		AcceptanceChecks: []string{"Tailwind classes render"},
	}
	err := validateStructuredProofPlan(plan, ledger)
	if err == nil {
		t.Fatal("expected memory-suggested proof plan to be rejected")
	}
	if !strings.Contains(err.Error(), "disallowed source") {
		t.Fatalf("expected disallowed-source error, got %v", err)
	}
}

func TestValidateStructuredProofPlanRequiresExecutableSignal(t *testing.T) {
	ledger := []StructuredObjective{{
		ID:       "document_install",
		Status:   "pending",
		Source:   structuredObjectiveSourceUserExplicit,
		Required: true,
	}}
	plan := StructuredProofPlan{
		ObjectiveID: "document_install",
		ProofType:   structuredProofTypeManualEvaluatorAcceptance,
		OutOfScope:  []string{"unrequested provider setup"},
	}
	err := validateStructuredProofPlan(plan, ledger)
	if err == nil {
		t.Fatal("expected proof plan without command or acceptance check to be rejected")
	}
	if !strings.Contains(err.Error(), "at least one executable command or acceptance check") {
		t.Fatalf("expected executable-signal error, got %v", err)
	}
}

func TestStructuredProofPolicyContainsTamperAndScopeRules(t *testing.T) {
	policy := strings.Join(structuredProofPolicy(), "\n")
	for _, want := range []string{
		"contract_first_tdd_loop",
		"allowed_objective_sources",
		"disallowed_scope_sources",
		"test_tampering",
		"allowed_test_corrections",
	} {
		if !strings.Contains(policy, want) {
			t.Fatalf("proof policy missing %q: %s", want, policy)
		}
	}
}

func TestHasStructuredProofPlanDetectsPartialContract(t *testing.T) {
	if hasStructuredProofPlan(StructuredProofPlan{}) {
		t.Fatal("empty proof plan should not be present")
	}
	if !hasStructuredProofPlan(StructuredProofPlan{ObjectiveID: "create_notes_crud"}) {
		t.Fatal("partial proof plan should be present")
	}
}
