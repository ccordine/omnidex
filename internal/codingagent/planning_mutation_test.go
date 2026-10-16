package codingagent

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func planningLedgerItem(id string, rank int64, state model.LedgerReviewState) model.LedgerItem {
	return model.LedgerItem{
		ID:         model.LedgerItemID(id),
		Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-1", Revision: 1},
		Rationale:  "required by the objective", Priority: model.LedgerPriorityNormal, Rank: rank,
		Disposition: model.LedgerDispositionActive, ReviewState: state,
	}
}

func planningLedgerFixture(state model.LedgerReviewState) EngineeringLedger {
	return EngineeringLedger{
		ID: "ledger-1", Revision: 4,
		Objectives:   []model.LedgerObjective{{LedgerItem: planningLedgerItem("objective", 0, state), Outcome: "ship"}},
		Requirements: []model.LedgerRequirement{{LedgerItem: planningLedgerItem("requirement", 1, state), Outcome: "works"}},
		Tasks: []model.LedgerTask{{
			LedgerItem: planningLedgerItem("task", 2, state), Goal: "implement",
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"},
		}},
		AcceptanceCriteria: []model.LedgerAcceptanceCriterion{{
			LedgerItem: planningLedgerItem("acceptance", 3, state), Outcome: "verified",
			RequirementIDs: []model.LedgerItemID{"requirement"}, TaskIDs: []model.LedgerItemID{"task"}, Required: true,
		}},
		Checks: []model.LedgerCheck{{
			LedgerItem: planningLedgerItem("check", 4, state), Argv: []string{"go", "test", "./..."},
			RequirementIDs:         []model.LedgerItemID{"requirement"},
			AcceptanceCriterionIDs: []model.LedgerItemID{"acceptance"}, Required: true,
		}},
	}
}

func TestPlanningMutationDirectActionUsesExactObservedRevision(t *testing.T) {
	turn := PlanningTurn{OperationID: "operation-1", OwnerTurnID: "turn-2", Text: "Remove the check.", ObservedRevision: 4}
	mutation, err := DirectPlanningMutation(turn, DirectPlanningAction{
		Kind: model.LedgerMutationRemove, TargetID: "check", Rationale: "owner selected remove",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutation.BaseRevision != 4 || mutation.OwnerTurnID != "turn-2" ||
		len(mutation.Operations) != 1 || mutation.Operations[0].TargetID != "check" {
		t.Fatalf("direct mutation lost exact authority: %#v", mutation)
	}
	turn.ObservedRevision = 0
	if _, err := DirectPlanningMutation(turn, DirectPlanningAction{Kind: model.LedgerMutationRemove, TargetID: "check", Rationale: "remove"}); err == nil {
		t.Fatal("direct action without an observed revision was accepted")
	}
}

func TestPlanningMutationAppliesTypedOperationWithoutRegeneratingLedger(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	constraint := model.LedgerConstraint{
		LedgerItem: planningLedgerItem("constraint", 5, model.LedgerReviewProposed),
		Rule:       "preserve identities",
	}
	mutation := LedgerMutation{
		OperationID: "operation-2", BaseRevision: ledger.Revision, OwnerTurnID: "turn-2",
		OwnerTurnDisposition: model.RepresentedOwnerTurn(
			[]model.LedgerItemID{"constraint"}),

		Operations: []model.LedgerMutationOperation{{
			Kind:      model.LedgerMutationAdd,
			Item:      &model.LedgerMutationItem{Constraint: &constraint},
			Rationale: "owner added a constraint",
		}},
	}
	next, err := ApplyPlanningMutation(ledger, mutation)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != ledger.Revision+1 || len(next.Constraints) != 1 ||
		next.Objectives[0] != ledger.Objectives[0] || next.Tasks[0].ID != ledger.Tasks[0].ID {
		t.Fatalf("typed mutation regenerated or lost untouched records: %#v", next)
	}
	if _, err := ApplyPlanningMutation(next, mutation); err == nil {
		t.Fatal("stale typed mutation was accepted")
	}
}

func TestPlanningMutationDirectRemovalCreatesReviewableIncompleteRevision(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	turn := PlanningTurn{OperationID: "remove-only-task", OwnerTurnID: "turn-remove", Text: "Remove task.", ObservedRevision: ledger.Revision}
	mutation, err := DirectPlanningMutation(turn, DirectPlanningAction{Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "explicit owner action"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := ApplyPlanningMutation(ledger, mutation)
	if err != nil {
		t.Fatalf("direct removal did not create a reviewable draft: %v", err)
	}
	if next.Tasks[0].Disposition != model.LedgerDispositionTombstoned || next.Revision != ledger.Revision+1 {
		t.Fatalf("direct removal result = %#v", next.Tasks[0])
	}
}
