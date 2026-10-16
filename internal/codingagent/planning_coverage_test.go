package codingagent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningLedgerContextIsBoundedAndDeterministic(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	for index := 0; index < 100; index++ {
		item := planningLedgerItem("constraint-"+string(rune(index+100)), int64(index+10), model.LedgerReviewAgreed)
		ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{LedgerItem: item, Rule: "bounded"})
	}
	first := BoundedPlanningLedger(ledger, []model.LedgerItemID{"task"}, 8)
	second := BoundedPlanningLedger(ledger, []model.LedgerItemID{"task"}, 8)
	if planningContextItemCount(first) > 8 {
		t.Fatalf("planning context count = %d, want at most 8", planningContextItemCount(first))
	}
	if len(first.Tasks) != 1 || first.Tasks[0].ID != "task" {
		t.Fatal("explicitly relevant task was omitted")
	}
	if planningContextItemCount(first) != planningContextItemCount(second) ||
		len(first.Constraints) != len(second.Constraints) {
		t.Fatal("bounded context selection was not deterministic")
	}
}

func TestPlanningAuditGatesButCannotApprove(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	audit := PlanningAuditReceipt{
		AuditID: "audit-1", Revision: ledger.Revision, Ready: true,
		Judgment: PlanningCoverageJudgment{
			Question: AuditPlanningQuestion, BaseRevision: ledger.Revision,
			Covered: true, Explanation: "all requirements have tasks, acceptance, and checks",
		},
	}
	if err := ValidatePlanningSignoff(ledger, audit); err != nil {
		t.Fatalf("ready agreed ledger rejected: %v", err)
	}
	if ledger.Objectives[0].ReviewState != model.LedgerReviewAgreed {
		t.Fatal("audit changed owner review state")
	}
	stale := audit
	stale.Revision--
	if err := ValidatePlanningSignoff(ledger, stale); err == nil {
		t.Fatal("stale planning audit allowed sign-off")
	}
	ledger.Tasks[0].ReviewState = model.LedgerReviewProposed
	if err := ValidatePlanningSignoff(ledger, audit); err == nil {
		t.Fatal("audit approved an item the owner had not agreed")
	}
}

func TestPlanningAuditExcludesSignoffControlFromSemanticScope(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	persistedTurnSeen := false
	controller, _ := fixtureController(t, fixtureWorkload(), func(assignment Assignment) (any, error) {
		for _, fact := range assignment.ConstructionFacts {
			if strings.Contains(fact.Quote, "Sign off the exact displayed planning revision") {
				t.Fatalf("planning audit treated sign-off control as engineering scope: %#v", assignment.ConstructionFacts)
			}
			persistedTurnSeen = persistedTurnSeen || strings.Contains(fact.Quote, "Build the requested module")
		}
		return PlanningCoverageJudgment{
			Question: AuditPlanningQuestion, BaseRevision: ledger.Revision, Covered: true,
			Explanation: "the persisted engineering turn is represented",
		}, nil
	}, passingChecks)
	turn := PlanningTurn{OperationID: "freeze-operation", OwnerTurnID: "freeze-turn",
		Text: "Sign off the exact displayed planning revision.", ObservedRevision: ledger.Revision}
	persisted := []PersistedPlanningTurn{{OwnerTurnID: "original-turn", ExactText: "Build the requested module",
		Dispositions: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective"}}}}}
	if _, err := controller.AuditPlanningLedger(context.Background(), "audit-signoff-scope", turn, ledger, persisted); err != nil {
		t.Fatal(err)
	}
	if !persistedTurnSeen {
		t.Fatal("planning audit omitted persisted engineering turns")
	}
}

func TestPlanningAuditUncoveredJudgmentIsReadOnly(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	judgment := PlanningCoverageJudgment{
		Question: AuditPlanningQuestion, BaseRevision: ledger.Revision,
		Covered: false, Explanation: "the owner's delivery instruction is missing",
		Suggestions: []string{"Ask the owner whether to add the omitted delivery instruction."},
	}
	if err := judgment.validate(AuditPlanningQuestion, ledger); err != nil {
		t.Fatalf("read-only uncovered audit rejected: %v", err)
	}
}

func TestPlanningAuditRejectsBlankSuggestions(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	judgment := PlanningCoverageJudgment{
		Question: AuditPlanningQuestion, BaseRevision: ledger.Revision, Covered: false,
		Explanation: "a concrete omission remains",
		Suggestions: []string{""},
	}
	if err := judgment.validate(AuditPlanningQuestion, ledger); err == nil {
		t.Fatal("blank planning suggestion was accepted")
	}
}

func TestPlanningCoverageReturnsSuggestionsWithoutMutationAuthority(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	controller, _ := fixtureController(t, fixtureWorkload(), func(assignment Assignment) (any, error) {
		if assignment.Role != PlanningCoverage {
			t.Fatalf("role = %s", assignment.Role)
		}
		if assignment.PlanningQuestionID != coveragePlanningQuestion || assignment.PlanningBaseRevision != ledger.Revision {
			t.Fatalf("coverage output authority was lost before the provider: %#v", assignment)
		}
		return PlanningCoverageJudgment{
			Question: coveragePlanningQuestion, BaseRevision: ledger.Revision, Covered: false, Explanation: "missing constraint",
			Suggestions: []string{"The owner can add the missing constraint in a new planning turn."},
		}, nil
	}, passingChecks)
	turn := PlanningTurn{OperationID: "owner-operation", OwnerTurnID: "owner-original", Text: "Add the constraint.", ObservedRevision: ledger.Revision}
	judgment, err := controller.AnalyzePlanningCoverage(context.Background(), turn, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if judgment.Covered || len(judgment.Suggestions) != 1 {
		t.Fatalf("read-only planning judgment = %#v", judgment)
	}
}

func TestPlanningCoverageReceivesDeclaredOutOfScopeDisposition(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	seen := false
	controller, _ := fixtureController(t, fixtureWorkload(), func(assignment Assignment) (any, error) {
		if assignment.Role != PlanningCoverage {
			t.Fatalf("role = %s", assignment.Role)
		}
		for _, fact := range assignment.ConstructionFacts {
			if fact.Kind == "declared_owner_turn_dispositions" && strings.Contains(fact.Quote, "explicitly deferred") {
				seen = true
			}
		}
		return PlanningCoverageJudgment{Question: coveragePlanningQuestion, BaseRevision: ledger.Revision, Covered: true, Explanation: "the declared clause is visibly out of scope"}, nil
	}, passingChecks)
	disposition := model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{{Kind: model.OwnerTurnOutOfScope, Reason: "explicitly deferred"}}}
	turn := PlanningTurn{OperationID: "coverage-disposition-operation", OwnerTurnID: "coverage-disposition-turn", Text: "Do not include deployment.", ObservedRevision: ledger.Revision}
	if _, err := controller.AnalyzePlanningCoverage(context.Background(), turn, ledger, disposition); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("coverage station did not receive the declared owner-turn disposition")
	}
	mutation := LedgerMutation{OwnerTurnDisposition: disposition}
	if !RequiresPlanningCoverageForTurn(mutation, false) {
		t.Fatal("natural-language out-of-scope turn skipped coverage")
	}
	if RequiresPlanningCoverageForTurn(mutation, true) {
		t.Fatal("deterministic non-structural direct control invoked semantic coverage")
	}
}

func TestBoundedPlanningLedgerNeverErasesOversizedExplicitContext(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	ids := []model.LedgerItemID{}
	for index := 0; index < 70; index++ {
		id := model.LedgerItemID(fmt.Sprintf("explicit-%02d", index))
		ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{LedgerItem: planningLedgerItem(string(id), int64(index+20), model.LedgerReviewAgreed), Rule: string(id)})
		ids = append(ids, id)
	}
	bounded := BoundedPlanningLedger(ledger, ids, 64)
	if count := planningContextItemCount(bounded); count == 0 || count > 64 {
		t.Fatalf("oversized explicit context count = %d", count)
	}
}
