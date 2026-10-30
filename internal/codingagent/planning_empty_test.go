package codingagent

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningQuestionSurvivesExistingTasksAndReplay(t *testing.T) {
	turn := PlanningTurn{OperationID: "question", Text: "Add reminders."}
	response := "Keep the inventory task. Should reminders work while the browser is closed? That choice affects the reminder implementation."
	journal := &durableTaskEvidence{entries: []Entry{
		{Phase: "planning_draft", Notes: []string{"old", "old request", "Superseded question."}},
		{Phase: "planning_draft", Notes: []string{turn.OperationID, turn.Text, response}},
	}}
	controller := Controller{Generation: 1, Evidence: journal}
	ledger := EngineeringLedger{ID: "1", Revision: 2, Tasks: []LedgerTask{
		{LedgerItem: LedgerItem{ID: "inventory", Rank: 1, Disposition: model.LedgerDispositionActive}, Goal: "Manage pantry inventory."},
	}}
	if err := controller.PublishPlanningLedger(t.Context(), turn, ledger); err != nil {
		t.Fatal(err)
	}
	got := journal.entries[2]
	if strings.Count(got.Notes[2], response) != 1 || !strings.Contains(got.Notes[2], "1. Manage pantry inventory.") ||
		strings.Contains(got.Notes[2], "Superseded question.") || got.PlanningProposal == nil || len(got.PlanningProposal.Tasks) != 1 {
		t.Fatalf("conversation or committed task order lost: %+v", got)
	}
	if err := controller.PublishPlanningLedger(t.Context(), turn, ledger); err != nil || len(journal.entries) != 3 {
		t.Fatal("replay duplicated the conversation", err)
	}
}

func TestEmptyPlanningReplyRetainsClarificationAndReplay(t *testing.T) {
	for _, question := range []string{"Which resource should I provision?", "Which records should the report include?"} {
		t.Run(question, func(t *testing.T) {
			turn := PlanningTurn{OperationID: "current", Text: "Create a new one."}
			journal := &durableTaskEvidence{entries: []Entry{
				{Phase: "planning_draft", Notes: []string{"old", "old request", "Old question."}},
				{Phase: "planning_draft", Notes: []string{turn.OperationID, turn.Text, question}},
			}}
			controller := Controller{Generation: 1, Evidence: journal}
			ledger := EngineeringLedger{ID: "1", Revision: 1, Tasks: []LedgerTask{{LedgerItem: LedgerItem{Disposition: model.LedgerDispositionTombstoned}, Goal: "Removed task"}}}
			if err := controller.PublishPlanningLedger(t.Context(), turn, ledger); err != nil {
				t.Fatal(err)
			}
			if got := journal.entries[2]; got.Notes[2] != question || got.PlanningProposal == nil {
				t.Fatalf("planning question replaced: %+v", got)
			}
			if err := controller.PublishPlanningLedger(t.Context(), turn, ledger); err != nil || len(journal.entries) != 3 {
				t.Fatal("replay duplicated the reply", err)
			}
		})
	}
}
