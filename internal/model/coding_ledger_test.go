package model

import (
	"strings"
	"testing"
)

func ledgerItem(id string, rank int64) LedgerItem {
	return LedgerItem{
		ID:          LedgerItemID(id),
		Provenance:  LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
		Rationale:   "required by the objective",
		Priority:    LedgerPriorityNormal,
		Rank:        rank,
		Disposition: LedgerDispositionActive,
		ReviewState: LedgerReviewAgreed,
	}
}

func validEngineeringLedger() EngineeringLedger {
	return EngineeringLedger{
		ID:           "ledger-1",
		Revision:     7,
		Objectives:   []LedgerObjective{{LedgerItem: ledgerItem("objective-1", 0), Outcome: "ship the contract"}},
		Requirements: []LedgerRequirement{{LedgerItem: ledgerItem("requirement-1", 1), Outcome: "preserve coverage"}},
		Constraints:  []LedgerConstraint{{LedgerItem: ledgerItem("constraint-1", 2), Rule: "do not flatten records"}},
		Groups:       []LedgerGroup{{LedgerItem: ledgerItem("group-1", 3), Title: "contracts"}},
		Tasks: []LedgerTask{{
			LedgerItem: ledgerItem("task-1", 4),
			Goal:       "implement contracts", GroupID: "group-1",
			RequirementIDs:         []LedgerItemID{"requirement-1"},
			AcceptanceCriterionIDs: []LedgerItemID{"acceptance-1"},
		}},
		AcceptanceCriteria: []LedgerAcceptanceCriterion{{
			LedgerItem: ledgerItem("acceptance-1", 5), Outcome: "contracts validate",
			RequirementIDs: []LedgerItemID{"requirement-1"}, TaskIDs: []LedgerItemID{"task-1"}, Required: true,
		}},
		Checks: []LedgerCheck{{
			LedgerItem: ledgerItem("check-1", 6), Argv: []string{"go", "test", "./..."},
			RequirementIDs:         []LedgerItemID{"requirement-1"},
			AcceptanceCriterionIDs: []LedgerItemID{"acceptance-1"}, Required: true,
		}},
		DecisionsAndQuestions: []LedgerDecisionQuestion{{
			LedgerItem: ledgerItem("decision-1", 7), Kind: LedgerDecision,
			Prompt: "Which route?", Answer: "frozen route", Affects: []LedgerItemID{"task-1"},
		}},
		Findings: []LedgerFinding{{
			LedgerItem: ledgerItem("finding-1", 8), Kind: LedgerFindingPlanning,
			Observation: "coverage was inspected", TargetIDs: []LedgerItemID{"task-1"},
		}},
	}
}

func TestEngineeringLedgerValidatesNormalizedRecords(t *testing.T) {
	ledger := validEngineeringLedger()
	if err := ledger.Validate(); err != nil {
		t.Fatalf("valid normalized ledger rejected: %v", err)
	}

	tombstone := ledgerItem("removed-constraint", 9)
	tombstone.Disposition = LedgerDispositionTombstoned
	tombstone.ReviewState = ""
	ledger.Constraints = append(ledger.Constraints, LedgerConstraint{LedgerItem: tombstone, Rule: "old rule"})
	if err := ledger.Validate(); err != nil {
		t.Fatalf("valid tombstone rejected: %v", err)
	}
}

func TestEngineeringLedgerRejectsMissingBindingsAndCycles(t *testing.T) {
	ledger := validEngineeringLedger()
	ledger.Checks[0].AcceptanceCriterionIDs = nil
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "explicit requirement and acceptance bindings") {
		t.Fatalf("expected check binding error, got %v", err)
	}

	ledger = validEngineeringLedger()
	ledger.Tasks = append(ledger.Tasks, LedgerTask{
		LedgerItem: ledgerItem("task-2", 9), Goal: "review contracts",
		RequirementIDs:         []LedgerItemID{"requirement-1"},
		AcceptanceCriterionIDs: []LedgerItemID{"acceptance-1"},
	})
	ledger.Dependencies = []LedgerDependency{
		{LedgerItem: ledgerItem("dependency-1", 10), TaskID: "task-1", PrerequisiteID: "task-2", Kind: LedgerDependencyRequires},
		{LedgerItem: ledgerItem("dependency-2", 11), TaskID: "task-2", PrerequisiteID: "task-1", Kind: LedgerDependencyRequires},
	}
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

func TestEngineeringLedgerRejectsRepeatedTaskFileGrants(t *testing.T) {
	for _, mutate := range []func(*LedgerTask){
		func(task *LedgerTask) { task.ReadFiles = []string{"source.go", "source.go"} },
		func(task *LedgerTask) { task.WriteFiles = []string{"source.go", "source.go"} },
		func(task *LedgerTask) { task.DeleteFiles = []string{"source.go", "source.go"} },
		func(task *LedgerTask) { task.ReadFiles, task.WriteFiles = []string{"source.go"}, []string{"source.go"} },
	} {
		ledger := validEngineeringLedger()
		mutate(&ledger.Tasks[0])
		if err := ledger.Validate(); err == nil {
			t.Fatal("repeated task file grant was accepted")
		}
	}
}

func TestLedgerMutationRequiresExactTypedDisposition(t *testing.T) {
	constraint := LedgerConstraint{LedgerItem: ledgerItem("constraint-2", 9), Rule: "keep history"}
	constraint.ReviewState = LedgerReviewProposed
	mutation := LedgerMutation{
		OperationID: "operation-1", BaseRevision: 7, OwnerTurnID: "turn-2",
		OwnerTurnDisposition: OwnerTurnDisposition{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{"constraint-2"}},
		Operations: []LedgerMutationOperation{{
			Kind: LedgerMutationAdd, Item: &LedgerMutationItem{Constraint: &constraint},
			Rationale: "the owner requested it",
		}},
	}
	if err := mutation.Validate(); err != nil {
		t.Fatalf("valid mutation rejected: %v", err)
	}
	mutation.Operations[0].Item.Requirement = &LedgerRequirement{}
	if err := mutation.Validate(); err == nil {
		t.Fatal("mutation with two typed payloads was accepted")
	}
}

func TestLedgerMutationValidatesPayloadIdentityAndOwnerReferences(t *testing.T) {
	mutation := LedgerMutation{
		OperationID: "operation-1", BaseRevision: 7, OwnerTurnID: "turn-2",
		OwnerTurnDisposition: OwnerTurnDisposition{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{""}},
		Operations: []LedgerMutationOperation{{
			Kind: LedgerMutationAdd, Item: &LedgerMutationItem{Requirement: &LedgerRequirement{}},
			Rationale: "the owner requested it",
		}},
	}
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "blank item reference") {
		t.Fatalf("expected blank represented item reference error, got %v", err)
	}

	mutation.OwnerTurnDisposition = OwnerTurnDisposition{Kind: OwnerTurnUnresolved, QuestionIDs: []LedgerItemID{""}}
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "blank item reference") {
		t.Fatalf("expected blank unresolved question reference error, got %v", err)
	}

	mutation.OwnerTurnDisposition = OwnerTurnDisposition{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{"requirement-2"}}
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "stable identity") {
		t.Fatalf("expected invalid typed payload error, got %v", err)
	}

	requirement := LedgerRequirement{LedgerItem: ledgerItem("requirement-3", 10), Outcome: "preserve identity"}
	mutation.Operations[0].Kind = LedgerMutationAmend
	mutation.Operations[0].TargetID = "requirement-2"
	mutation.Operations[0].Item = &LedgerMutationItem{Requirement: &requirement}
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "does not match target") {
		t.Fatalf("expected amend identity mismatch error, got %v", err)
	}
}

func TestEngineeringLedgerRejectsUnknownFindingKind(t *testing.T) {
	ledger := validEngineeringLedger()
	ledger.Findings[0].Kind = LedgerFindingKind("not_a_kind")
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("expected invalid finding kind error, got %v", err)
	}
}

func validLedgerMutationOperation(kind LedgerMutationKind) LedgerMutation {
	constraint := LedgerConstraint{LedgerItem: ledgerItem("constraint-2", 9), Rule: "keep history"}
	operation := LedgerMutationOperation{Kind: kind, Rationale: "the owner requested it"}
	switch kind {
	case LedgerMutationAdd:
		constraint.ReviewState = LedgerReviewProposed
		operation.Item = &LedgerMutationItem{Constraint: &constraint}
	case LedgerMutationAmend:
		operation.TargetID = constraint.ID
		operation.Item = &LedgerMutationItem{Constraint: &constraint}
	case LedgerMutationRemove, LedgerMutationRestore:
		operation.TargetID = constraint.ID
	case LedgerMutationReview:
		operation.TargetID = constraint.ID
		operation.ReviewState = LedgerReviewAgreed
	case LedgerMutationReorder:
		rank := int64(3)
		operation.TargetID = constraint.ID
		operation.Rank = &rank
	}
	return LedgerMutation{
		OperationID:          "operation-variant",
		BaseRevision:         7,
		OwnerTurnID:          "turn-2",
		OwnerTurnDisposition: OwnerTurnDisposition{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{constraint.ID}},
		Operations:           []LedgerMutationOperation{operation},
	}
}

func TestLedgerMutationRejectsBlankTargets(t *testing.T) {
	for _, kind := range []LedgerMutationKind{LedgerMutationRemove, LedgerMutationRestore, LedgerMutationReview, LedgerMutationReorder} {
		mutation := validLedgerMutationOperation(kind)
		mutation.Operations[0].TargetID = " "
		if err := mutation.Validate(); err == nil {
			t.Errorf("%s mutation with blank target was accepted", kind)
		}
	}
}

func TestLedgerMutationRejectsMixedOwnerTurnDisposition(t *testing.T) {
	mutation := validLedgerMutationOperation(LedgerMutationAdd)
	mutation.OwnerTurnDisposition.QuestionIDs = []LedgerItemID{"question-1"}
	if err := mutation.Validate(); err == nil {
		t.Fatal("represented disposition with unresolved questions was accepted")
	}

	mutation.OwnerTurnDisposition = OwnerTurnDisposition{
		Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{"constraint-2"}, Reason: " ",
	}
	if err := mutation.Validate(); err == nil {
		t.Fatal("represented disposition with an inactive reason field was accepted")
	}

	mutation.OwnerTurnDisposition = OwnerTurnDisposition{
		Kind: OwnerTurnOutOfScope, Reason: "unrelated request", ItemIDs: []LedgerItemID{"constraint-2"},
	}
	if err := mutation.Validate(); err == nil {
		t.Fatal("out-of-scope disposition with represented items was accepted")
	}

	mutation.OwnerTurnDisposition = OwnerTurnDisposition{
		Kind: OwnerTurnUnresolved, QuestionIDs: []LedgerItemID{"question-1"}, Reason: "also out of scope",
	}
	if err := mutation.Validate(); err == nil {
		t.Fatal("unresolved disposition with an out-of-scope reason was accepted")
	}
}

func TestLedgerMutationRejectsFieldsFromOtherOperationVariants(t *testing.T) {
	rank := int64(2)
	testCases := []struct {
		name   string
		mutate func(*LedgerMutationOperation)
	}{
		{name: "add target", mutate: func(operation *LedgerMutationOperation) { operation.TargetID = "constraint-2" }},
		{name: "add blank target", mutate: func(operation *LedgerMutationOperation) { operation.TargetID = " " }},
		{name: "add review", mutate: func(operation *LedgerMutationOperation) { operation.ReviewState = LedgerReviewAgreed }},
		{name: "add rank", mutate: func(operation *LedgerMutationOperation) { operation.Rank = &rank }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mutation := validLedgerMutationOperation(LedgerMutationAdd)
			testCase.mutate(&mutation.Operations[0])
			if err := mutation.Validate(); err == nil {
				t.Fatal("mutation with fields from another operation variant was accepted")
			}
		})
	}
}

func TestEngineeringLedgerRejectsActiveRelationshipsToTombstones(t *testing.T) {
	ledger := validEngineeringLedger()
	ledger.AcceptanceCriteria[0].Disposition = LedgerDispositionTombstoned
	ledger.AcceptanceCriteria[0].ReviewState = ""
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "active acceptance criterion") {
		t.Fatalf("expected active task to tombstoned acceptance error, got %v", err)
	}

	ledger = validEngineeringLedger()
	tombstonedTask := LedgerTask{
		LedgerItem:             ledgerItem("task-2", 9),
		Goal:                   "retired prerequisite",
		RequirementIDs:         []LedgerItemID{"requirement-1"},
		AcceptanceCriterionIDs: []LedgerItemID{"acceptance-1"},
	}
	tombstonedTask.Disposition = LedgerDispositionTombstoned
	tombstonedTask.ReviewState = ""
	ledger.Tasks = append(ledger.Tasks, tombstonedTask)
	ledger.Dependencies = append(ledger.Dependencies, LedgerDependency{
		LedgerItem: ledgerItem("dependency-1", 10),
		TaskID:     "task-1", PrerequisiteID: "task-2", Kind: LedgerDependencyRequires,
	})
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "active task task-2") {
		t.Fatalf("expected active dependency to tombstoned prerequisite error, got %v", err)
	}
}

func TestLedgerMutationRequiresAddedItemsToStartActiveAndProposed(t *testing.T) {
	mutation := validLedgerMutationOperation(LedgerMutationAdd)
	mutation.Operations[0].Item.Constraint.ReviewState = LedgerReviewAgreed
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "active and proposed") {
		t.Fatalf("expected agreed add payload error, got %v", err)
	}

	mutation.Operations[0].Item.Constraint.Disposition = LedgerDispositionTombstoned
	mutation.Operations[0].Item.Constraint.ReviewState = ""
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "active and proposed") {
		t.Fatalf("expected tombstoned add payload error, got %v", err)
	}
}

func TestLedgerMutationRepresentsOutOfScopeTurnWithoutOperations(t *testing.T) {
	mutation := LedgerMutation{
		OperationID:          "operation-out-of-scope",
		BaseRevision:         7,
		OwnerTurnID:          "turn-2",
		OwnerTurnDisposition: OwnerTurnDisposition{Kind: OwnerTurnOutOfScope, Reason: "unrelated to this ledger"},
	}
	if err := mutation.Validate(); err != nil {
		t.Fatalf("valid out-of-scope disposition rejected: %v", err)
	}

	mutation.Operations = validLedgerMutationOperation(LedgerMutationAdd).Operations
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "operations from another disposition") {
		t.Fatalf("expected out-of-scope operation error, got %v", err)
	}
}

func TestLedgerMutationSupportsPerStatementMixedDispositions(t *testing.T) {
	mutation := LedgerMutation{
		OperationID: "mixed-turn", BaseRevision: 2, OwnerTurnID: "owner-mixed",
		OwnerTurnDisposition: OwnerTurnDisposition{Statements: []OwnerStatementDisposition{
			{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{"constraint-2"}},
			{Kind: OwnerTurnUnresolved, QuestionIDs: []LedgerItemID{"question-2"}},
			{Kind: OwnerTurnOutOfScope, Reason: "deployment belongs to another objective"},
		}},
		Operations: []LedgerMutationOperation{{Kind: LedgerMutationRemove, TargetID: "constraint-2", Rationale: "owner removed it"}},
	}
	if err := mutation.Validate(); err != nil {
		t.Fatalf("mixed per-statement disposition rejected: %v", err)
	}
}

func TestLedgerMutationRejectsExactDuplicateOperations(t *testing.T) {
	mutation := validLedgerMutationOperation(LedgerMutationAdd)
	mutation.Operations = append(mutation.Operations, mutation.Operations[0])
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "exact duplicate operation") {
		t.Fatalf("expected duplicate operation error, got %v", err)
	}
}

func TestLedgerChecksRejectBlankCommandExecutables(t *testing.T) {
	ledger := validEngineeringLedger()
	ledger.Checks[0].Argv = []string{" "}
	if err := ledger.Validate(); err == nil || !strings.Contains(err.Error(), "nonblank command executable") {
		t.Fatalf("expected aggregate blank executable error, got %v", err)
	}

	check := ledger.Checks[0]
	check.ReviewState = LedgerReviewProposed
	mutation := LedgerMutation{
		OperationID: "operation-blank-check", BaseRevision: ledger.Revision, OwnerTurnID: "turn-2",
		OwnerTurnDisposition: OwnerTurnDisposition{Kind: OwnerTurnRepresented, ItemIDs: []LedgerItemID{check.ID}},
		Operations: []LedgerMutationOperation{{
			Kind: LedgerMutationAdd, Item: &LedgerMutationItem{Check: &check}, Rationale: "owner requested it",
		}},
	}
	if err := mutation.Validate(); err == nil || !strings.Contains(err.Error(), "nonblank command executable") {
		t.Fatalf("expected mutation blank executable error, got %v", err)
	}
}
