package codingagent

import "testing"

func TestPlanningReplayUsesTheCurrentOwnerDirection(t *testing.T) {
	turn := PlanningTurn{OperationID: "queued", Text: "Retain the fixture."}
	journal := &durableTaskEvidence{entries: []Entry{{Phase: "planning_draft",
		Notes: []string{turn.OperationID, turn.Text, "Old fixture amendment."}}}}
	provider := &directPlanningProvider{t: t, source: "The newer direction confirms the fixture was already removed. No amendment."}
	route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
	controller := Controller{JobID: 1, Generation: 2, Root: t.TempDir(), Evidence: journal,
		Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": provider},
		Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
	if response, err := controller.planningResponse(t.Context(), turn, nil); err != nil || response != "Old fixture amendment." || len(provider.roles) != 0 {
		t.Fatalf("unchanged replay called the provider: %q %v", response, err)
	}
	turn.NewerOwnerDirection = &PlanningDirection{OperationID: "latest", Text: "Review is complete and the fixture was removed."}
	if response, err := controller.planningResponse(t.Context(), turn, nil); err != nil || response != provider.source || len(provider.roles) != 1 {
		t.Fatalf("new direction reused an obsolete answer: %q %v", response, err)
	}
	if response, err := controller.planningResponse(t.Context(), turn, nil); err != nil || response != provider.source || len(provider.roles) != 1 {
		t.Fatalf("current direction did not replay: %q %v", response, err)
	}
	if journal.entries[0].Notes[2] != "Old fixture amendment." {
		t.Fatal("prior semantic evidence was discarded")
	}
}
