package codingagent

import (
	"context"
	"fmt"
	"strings"
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

func TestInitialPlanningContractCorrectionIsBoundedAndCodeDirected(t *testing.T) {
	valid := planningLedgerFixture(model.LedgerReviewProposed)
	valid.Revision = 1
	validDisposition := model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented,
		ItemIDs: []model.LedgerItemID{"objective", "requirement", "task", "acceptance", "check"}}
	invalid := valid
	invalid.Checks = nil
	attempts := 0
	controller, _ := fixtureController(t, fixtureWorkload(), func(assignment Assignment) (any, error) {
		attempts++
		if attempts == 1 {
			if len(assignment.ValidationNotes) != 0 {
				t.Fatalf("first proposal received correction notes: %#v", assignment.ValidationNotes)
			}
			return InitialPlanningProposal{Ledger: invalid, OwnerTurnDisposition: validDisposition}, nil
		}
		if attempts != 2 || !strings.Contains(strings.Join(assignment.ValidationNotes, " "), "requires an active check") {
			t.Fatalf("replacement did not receive the exact validator error: %#v", assignment.ValidationNotes)
		}
		return InitialPlanningProposal{Ledger: valid, OwnerTurnDisposition: validDisposition}, nil
	}, passingChecks)
	turn := PlanningTurn{OperationID: "initial-operation", OwnerTurnID: "initial-turn", Text: "Ship it.", ObservedRevision: 0}
	proposal, err := controller.ProposePlanningLedger(context.Background(), turn)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(proposal.Ledger.Checks) != 1 {
		t.Fatalf("initial contract correction attempts=%d proposal=%#v", attempts, proposal)
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
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"constraint"},
		},
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

func TestPlanningMutationStructuralCheckpointClassification(t *testing.T) {
	turn := PlanningTurn{OperationID: "operation-3", OwnerTurnID: "turn-3", Text: "Agree.", ObservedRevision: 4}
	review, err := DirectPlanningMutation(turn, DirectPlanningAction{
		Kind: model.LedgerMutationReview, TargetID: "task",
		ReviewState: model.LedgerReviewAgreed, Rationale: "owner agreed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if RequiresPlanningCoverage(review) {
		t.Fatal("review-state-only action unnecessarily invoked semantic coverage")
	}
	remove, err := DirectPlanningMutation(turn, DirectPlanningAction{
		Kind: model.LedgerMutationRemove, TargetID: "task", Rationale: "owner removed task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !RequiresPlanningCoverage(remove) {
		t.Fatal("structural task removal skipped semantic coverage")
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

func TestPlanningMutationRelevantSelectionRetainsNamedRemovedHighRankItem(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	removed := model.LedgerConstraint{LedgerItem: planningLedgerItem("removed-target", 10_000, model.LedgerReviewAgreed), Rule: "restore me"}
	removed.Disposition, removed.ReviewState = model.LedgerDispositionTombstoned, ""
	ledger.Constraints = append(ledger.Constraints, removed)
	for index := 0; index < 80; index++ {
		ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{LedgerItem: planningLedgerItem(fmt.Sprintf("ordinary-%d", index), int64(index+10), model.LedgerReviewAgreed), Rule: "ordinary"})
	}
	ids := PlanningRelevantItemIDs(ledger, "Please restore removed-target exactly.")
	bounded := BoundedPlanningLedger(ledger, ids, 8)
	found := false
	for _, constraint := range bounded.Constraints {
		found = found || constraint.ID == "removed-target"
	}
	if !found {
		t.Fatalf("explicit removed target was trimmed: %#v", bounded.Constraints)
	}
}

func TestPlanningMutationRelevantSelectionResolvesDisplayedOutcomeBeforeBounding(t *testing.T) {
	ledger := planningLedgerFixture(model.LedgerReviewAgreed)
	ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{
		LedgerItem: planningLedgerItem("high-rank-cache-rule", 50_000, model.LedgerReviewAgreed),
		Rule:       "Preserve the accepted cache key across restarts",
	})
	for index := 0; index < 80; index++ {
		ledger.Constraints = append(ledger.Constraints, model.LedgerConstraint{
			LedgerItem: planningLedgerItem(fmt.Sprintf("lower-rank-%d", index), int64(index+10), model.LedgerReviewAgreed),
			Rule:       fmt.Sprintf("ordinary rule %d", index),
		})
	}
	targetReachedMutation := false
	controller, _ := fixtureController(t, fixtureWorkload(), func(assignment Assignment) (any, error) {
		switch assignment.Role {
		case PlanningTarget:
			selected := []model.LedgerItemID{}
			for _, fact := range assignment.ConstructionFacts {
				if fact.Kind == "bounded_planning_context" && strings.Contains(fact.Quote, `"high-rank-cache-rule"`) {
					selected = append(selected, "high-rank-cache-rule")
				}
			}
			return PlanningTargetSelection{Question: targetPlanningQuestion, BaseRevision: ledger.Revision, ItemIDs: selected}, nil
		case PlanningMutation:
			for _, fact := range assignment.ConstructionFacts {
				if fact.Kind == "bounded_planning_context" && strings.Contains(fact.Quote, `"high-rank-cache-rule"`) {
					targetReachedMutation = true
				}
			}
			return LedgerMutation{
				OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"high-rank-cache-rule"}},
				Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "high-rank-cache-rule", ReviewState: model.LedgerReviewNeedsDiscussion, Rationale: "owner referenced this rule"}},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected role %s", assignment.Role)
		}
	}, passingChecks)
	turn := PlanningTurn{OperationID: "semantic-target-operation", OwnerTurnID: "semantic-target-turn", Text: "Please amend the API rule.", ObservedRevision: ledger.Revision}
	if _, err := controller.PlanPlanningMutation(context.Background(), ledger, turn); err != nil {
		t.Fatal(err)
	}
	if !targetReachedMutation {
		t.Fatal("semantically resolved high-rank target was omitted from the bounded mutation context")
	}
}
