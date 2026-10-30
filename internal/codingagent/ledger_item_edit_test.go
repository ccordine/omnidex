package codingagent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestManualLedgerTaskEditPreservesCodeOwnedFields(t *testing.T) {
	task := packetFixture()
	ledger := EngineeringLedger{ID: "ledger", Revision: 3, Tasks: []LedgerTask{task}}
	turn := PlanningTurn{OperationID: "manual-edit", OwnerTurnID: "owner-edit", Text: "Change the name.", ObservedRevision: 3}
	draft := task
	draft.Goal = "Renamed item"
	draft.ID = "foreign"
	draft.Rank = 999
	mutation, err := DirectPlanningItemMutation(ledger, turn, DirectPlanningAction{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &draft})
	if err != nil {
		t.Fatal(err)
	}
	result := mutation.Operations[0].Item.Task
	if !reflect.DeepEqual(result.LedgerItem, task.LedgerItem) || result.Goal != "Renamed item" || !reflect.DeepEqual(result.Acceptance, task.Acceptance) {
		t.Fatal("manual edit changed task identity, order or retained acceptance")
	}
	created, err := DirectPlanningItemMutation(ledger, turn, DirectPlanningAction{Kind: model.LedgerMutationAdd, Item: &model.LedgerTask{Goal: "New item", OwnerContext: []model.TaskContext{{Text: "Relevant context"}}, Acceptance: []model.TaskAcceptance{{Text: "Verify the new behavior"}}}})
	if err != nil {
		t.Fatal(err)
	}
	item := created.Operations[0].Item.Task
	if item.ID != "manual-manual-edit" || item.OwnerContext[0].Provenance.Reference != turn.OwnerTurnID || item.Acceptance[0].ID == "" {
		t.Fatal("manual creation lost code-owned identity or provenance")
	}
}

func TestManualLedgerOrderMovesWithoutRewritingTaskContents(t *testing.T) {
	a, b, c := packetFixture(), packetFixture(), packetFixture()
	a.ID, a.Rank = "a", 0
	b.ID, b.Rank = "b", 10
	c.ID, c.Rank = "c", 20
	a.Goal, b.Goal, c.Goal = "First", "Second", "Third"
	ledger := EngineeringLedger{ID: "ledger", Revision: 3, Tasks: []LedgerTask{a, b, c}}
	mutation, err := DirectPlanningItemMutation(ledger, PlanningTurn{OperationID: "move", OwnerTurnID: "move", Text: "Move second above first.", ObservedRevision: 3}, DirectPlanningAction{Kind: model.LedgerMutationReorder, TargetID: "b", Move: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(mutation.Operations) != 2 {
		t.Fatal("moving neighbors rewrote other items")
	}
	for _, op := range mutation.Operations {
		if op.Item != nil || op.Kind != model.LedgerMutationReorder || op.TargetID == "c" {
			t.Fatal("order control rewrote task contents")
		}
		if op.TargetID == "b" && *op.Rank != 0 {
			t.Fatal("selected item did not move first")
		}
	}
}

type itemPlanningProvider struct {
	t             *testing.T
	calls         []Assignment
	wantPreceding []model.LedgerItemID
}

func (p *itemPlanningProvider) Run(_ context.Context, call Assignment, _ func(NativeEvent) error) (Result, error) {
	p.calls = append(p.calls, call)
	if len(call.PlanningLedger.Tasks) != 1 || call.PlanningLedger.Tasks[0].ID != "selected" {
		p.t.Fatal("planner received sibling ledger items")
	}
	var ids []model.LedgerItemID
	for _, item := range call.PrecedingIncompleteTasks {
		ids = append(ids, item.ID)
	}
	if !reflect.DeepEqual(ids, p.wantPreceding) {
		p.t.Fatalf("wrong preceding work: %v, want %v", ids, p.wantPreceding)
	}
	raw, _ := json.Marshal(PromptContext(call))
	if strings.Contains(string(raw), "SIBLING_SECRET") || strings.Contains(string(raw), "GLOBAL_HISTORY") || strings.Contains(string(raw), "EXCLUDED_CONTEXT") {
		p.t.Fatal("planner received unrelated context")
	}
	content := json.RawMessage(`"Updated the selected item."`)
	if call.Role == PlanningMutation {
		task := call.PlanningLedger.Tasks[0]
		task.Objective = "Revised only this task"
		// The parser's target is mechanical; the selected identity stays code-owned.
		task.ID = "model-chosen-id"
		content, _ = json.Marshal(map[string]any{"operations": []model.LedgerMutationOperation{{Kind: model.LedgerMutationAmend, TargetID: "model-chosen-id", Item: &model.LedgerMutationItem{Task: &task}}}})
	}
	return Result{Content: content, Provider: call.Route.Provider, Model: call.Route.Model, Completed: true}, nil
}

func TestSelectedItemPlanningUsesPrecedingIncompleteOutcomes(t *testing.T) {
	makeTask := func(id string, rank int64) LedgerTask {
		task := packetFixture()
		task.ID, task.Rank, task.Goal, task.Objective = model.LedgerItemID(id), rank, id, id+" outcome"
		task.OwnerContext = []model.TaskContext{{Text: "EXCLUDED_CONTEXT", Provenance: task.Provenance}}
		task.Acceptance = []model.TaskAcceptance{{ID: "accept", Text: id + " works", Verification: []string{"EXCLUDED_CONTEXT"}}}
		return task
	}
	selected := makeTask("selected", 30)
	selected.OwnerContext, selected.Acceptance = nil, nil
	earlier := makeTask("earlier", 10)
	tiedBefore := makeTask("aaa", 30)
	accepted := makeTask("accepted", 0)
	later := makeTask("zzz", 30)
	later.Goal, later.Objective, accepted.Goal, accepted.Objective = "EXCLUDED_CONTEXT", "EXCLUDED_CONTEXT", "EXCLUDED_CONTEXT", "EXCLUDED_CONTEXT"
	removed := makeTask("removed", 5)
	removed.Goal, removed.Objective, removed.Disposition = "EXCLUDED_CONTEXT", "EXCLUDED_CONTEXT", model.LedgerDispositionTombstoned
	removed.ReviewState = ""
	ledger := EngineeringLedger{ID: "ledger", Revision: 3, Tasks: []LedgerTask{later, selected, removed, tiedBefore, accepted, earlier}}
	p := &itemPlanningProvider{t: t, wantPreceding: []model.LedgerItemID{"earlier", "aaa"}}
	route := Route{Provider: "fixture", Model: "fixture"}
	c := Controller{JobID: 1, Generation: 2, Root: "/fixture", CompletedTaskIDs: []string{"accepted"}, Evidence: &durableTaskEvidence{}, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}}
	mutation, err := c.PlanPlanningMutation(t.Context(), ledger, PlanningTurn{ItemID: "selected", OperationID: "edit", OwnerTurnID: "owner-edit", Text: "Build a follow-on feature using the earlier outcome.", ObservedRevision: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 2 {
		t.Fatal("preceding context added a model stage")
	}
	for _, call := range p.calls {
		raw, _ := json.Marshal(PromptContext(call))
		for _, text := range []string{"earlier outcome", "earlier works", "aaa outcome", "read-only"} {
			if !strings.Contains(string(raw), text) {
				t.Fatalf("%s lost prerequisite meaning: %s", call.Role, text)
			}
		}
	}
	next, err := ApplyPlanningMutation(ledger, mutation)
	if err != nil {
		t.Fatal(err)
	}
	for i, task := range ledger.Tasks {
		if task.ID != "selected" && !reflect.DeepEqual(task, next.Tasks[i]) {
			t.Fatal("read-only predecessor or sibling was changed")
		}
		if task.ID == "selected" && len(next.Tasks[i].OwnerContext) != 0 {
			t.Fatal("predecessor context was copied into the execution item")
		}
	}
}

func TestSelectedItemPlanningReceivesOnlyItsConversation(t *testing.T) {
	selected := packetFixture()
	selected.ID = "selected"
	sibling := packetFixture()
	sibling.ID = "sibling"
	sibling.Goal = "SIBLING_SECRET"
	ledger := EngineeringLedger{ID: "ledger", Revision: 3, Tasks: []LedgerTask{selected, sibling}}
	journal := &durableTaskEvidence{entries: []Entry{
		{Phase: "planning_draft", Notes: []string{"global", "GLOBAL_HISTORY", "Global response"}},
		{Phase: "planning_draft", TaskID: "sibling", Notes: []string{"sibling", "SIBLING_SECRET", "Other response"}},
		{Phase: "planning_draft", TaskID: "selected", Notes: []string{"previous", "Previous selected request", "Previous selected reply"}},
	}}
	p := &itemPlanningProvider{t: t}
	route := Route{Provider: "fixture", Model: "fixture"}
	c := Controller{JobID: 1, Generation: 2, Root: "/fixture", Evidence: journal, Environment: durableTaskEnvironment{}, Providers: map[string]Provider{"fixture": p}, Configuration: Configuration{Mode: Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}}
	mutation, err := c.PlanPlanningMutation(t.Context(), ledger, PlanningTurn{ItemID: "selected", OperationID: "edit", OwnerTurnID: "owner-edit", Text: "Revise the instructions.", ObservedRevision: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 2 || len(p.calls[0].PlanningConversation) != 2 || p.calls[0].PlanningConversation[0].Text != "Previous selected request" {
		t.Fatal("selected-item conversation was not retained independently")
	}
	if len(mutation.Operations) != 1 || mutation.Operations[0].TargetID != "selected" || mutation.Operations[0].Item.Task.ID != "selected" {
		t.Fatal("model changed the selected target")
	}
	for _, entry := range journal.entries {
		if entry.Phase == "planning_draft" && len(entry.Notes) > 0 && entry.Notes[0] == "edit" && entry.TaskID != "selected" {
			t.Fatal("conversation attached to another item")
		}
	}
	if err := c.PublishPlanningLedger(t.Context(), PlanningTurn{ItemID: "selected", OperationID: "edit", Text: "Revise the instructions."}, ledger); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(journal.entries[len(journal.entries)-1].Notes[2], "SIBLING_SECRET") {
		t.Fatal("selected-item reply dumped the sibling roadmap")
	}
}
