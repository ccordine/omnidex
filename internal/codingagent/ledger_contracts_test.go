package codingagent

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestLedgerRouteConfigurationIsExplicitAndRevisioned(t *testing.T) {
	route := Route{Provider: "codex", Model: "gpt-5.6-sol", Effort: "medium", ServiceTier: "priority"}
	configuration := RouteConfigurationRevision{
		ID: "routes-2", Revision: 2, BaseRevision: 1,
		LedgerID: "ledger-1", LedgerRevision: 9, OwnerTurnID: "turn-8",
		State:  RouteConfigurationFrozen,
		Routes: RouteSet{Planner: route, Executor: route, Reviewer: route},
	}
	if err := configuration.Validate(); err != nil {
		t.Fatalf("valid route configuration rejected: %v", err)
	}
	configuration.Routes.Reviewer.Model = ""
	if err := configuration.Validate(); err == nil {
		t.Fatal("configuration without an explicit reviewer route was accepted")
	}
}

func TestLedgerFrozenRouteConfigurationRequiresExplicitOptions(t *testing.T) {
	route := Route{Provider: "codex", Model: "gpt-5.6-sol", Effort: "medium", ServiceTier: "priority"}
	valid := RouteConfigurationRevision{
		ID: "routes-1", Revision: 1, BaseRevision: 0,
		LedgerID: "ledger-1", LedgerRevision: 9,
		State:  RouteConfigurationFrozen,
		Routes: RouteSet{Planner: route, Executor: route, Reviewer: route},
	}

	testCases := []struct {
		name   string
		mutate func(*RouteConfigurationRevision)
	}{
		{name: "planner effort", mutate: func(configuration *RouteConfigurationRevision) { configuration.Routes.Planner.Effort = "" }},
		{name: "executor service tier", mutate: func(configuration *RouteConfigurationRevision) { configuration.Routes.Executor.ServiceTier = "" }},
		{name: "reviewer effort", mutate: func(configuration *RouteConfigurationRevision) { configuration.Routes.Reviewer.Effort = "" }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := valid
			testCase.mutate(&configuration)
			if err := configuration.Validate(); err == nil || !strings.Contains(err.Error(), "requires explicit reasoning effort and service tier") {
				t.Fatalf("expected explicit frozen route option error, got %v", err)
			}
		})
	}

	proposed := valid
	proposed.State = RouteConfigurationProposed
	proposed.Routes.Planner.Effort = ""
	if err := proposed.Validate(); err != nil {
		t.Fatalf("proposed route configuration should permit unresolved options: %v", err)
	}
}

func TestLedgerMutationRejectsStaleBaseRevision(t *testing.T) {
	constraint := model.LedgerConstraint{
		LedgerItem: model.LedgerItem{
			ID:         "constraint-1",
			Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "turn-2", Revision: 3},
			Rationale:  "owner requirement", Priority: model.LedgerPriorityNormal,
			Disposition: model.LedgerDispositionActive, ReviewState: model.LedgerReviewProposed,
		},
		Rule: "preserve history",
	}
	mutation := LedgerMutation{
		OperationID: "operation-1", BaseRevision: 3, OwnerTurnID: "turn-2",
		OwnerTurnDisposition: model.OwnerTurnDisposition{
			Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"constraint-1"},
		},
		Operations: []model.LedgerMutationOperation{{
			Kind:      model.LedgerMutationAdd,
			Item:      &model.LedgerMutationItem{Constraint: &constraint},
			Rationale: "owner requested it",
		}},
	}
	if err := ValidateLedgerMutation(4, mutation); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale mutation error, got %v", err)
	}
	if err := ValidateLedgerMutation(3, mutation); err != nil {
		t.Fatalf("current mutation rejected: %v", err)
	}
}
