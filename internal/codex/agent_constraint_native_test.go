package codex

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestInstalledProjectConstraintBecomesTaskGuidance(t *testing.T) {
	client, selectedModel := installedAgentClient(t)
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			fixture := capturedPlanningFixture(t, name)
			turn := codingagent.PlanningTurn{OperationID: "project-constraint", OwnerTurnID: "owner-constraint", ObservedRevision: fixture.Ledger.Revision, Text: "Deliver this project locally."}
			response := "Add project-wide constraint new-constraint: delivery remains local in the existing directory; no repository initialization, commit, push or PR is required. This applies to every remaining task. Preserve their other fields, requirements, acceptance criteria and verification."
			journal := &replayPlanningJournal{entries: []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, response}}}}
			route := codingagent.Route{Provider: "codex", Model: selectedModel, Effort: "low", ServiceTier: "default"}
			controller := codingagent.Controller{JobID: 17, Generation: 1, Root: t.TempDir(),
				Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
				Providers:     map[string]codingagent.Provider{"codex": replayPlanningProvider{client: client, t: t}}, Evidence: journal, Environment: replayPlanningEnvironment{},
				Requirements: []codingagent.Requirement{{ID: "objective", Outcome: fixture.Instruction}}}
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			mutation, err := controller.PlanPlanningMutation(ctx, fixture.Ledger, turn)
			var failure *codingagent.ParserFailure
			if errors.As(err, &failure) {
				mutation, err = controller.PlanPlanningMutation(ctx, fixture.Ledger, turn)
			}
			if err != nil {
				t.Fatal(err)
			}
			next, err := codingagent.ApplyPlanningMutation(fixture.Ledger, mutation)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, constraint := range next.Constraints {
				if constraint.ID == "new-constraint" {
					found = true
					if constraint.ReviewState != model.LedgerReviewProposed || constraint.Disposition != model.LedgerDispositionActive {
						t.Fatalf("new constraint bypassed owner review: %+v", constraint)
					}
				}
			}
			if !found {
				t.Fatal("new project constraint was lost")
			}
			for index, before := range fixture.Ledger.Tasks {
				after := next.Tasks[index]
				if before.Disposition != model.LedgerDispositionActive {
					continue
				}
				bound := false
				for _, id := range after.ConstraintIDs {
					bound = bound || id == "new-constraint"
				}
				if !bound {
					t.Fatalf("task %s lost project policy", before.ID)
				}
				after.ConstraintIDs, after.ReviewState = before.ConstraintIDs, before.ReviewState
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("constraint binding changed unrelated task fields: before=%+v after=%+v", before, after)
				}
			}
			t.Logf("project constraint initialized by code and bound to %d unchanged tasks", len(next.Tasks))
		})
	}
}
