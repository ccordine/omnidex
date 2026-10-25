package codingagent

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

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
