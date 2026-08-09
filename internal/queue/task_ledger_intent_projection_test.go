package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestAcceptedIntentProjectionUsesInternalIdentityAndNoStepAssignment(t *testing.T) {
	intent := artifacts.IntentArtifact{
		UserGoal: "Build safely", Mode: "execute", MemoryMode: artifacts.MemoryModeOff,
		RequiresAction: true,
		Objectives: []artifacts.Objective{{
			ID: "goal:root", Description: "Implement the accepted change", Priority: 90,
			RequiresAction: true, AcceptanceCriteria: []string{"Focused proof passes"},
		}},
		Constraints:        []string{"Preserve existing behavior"},
		CompletionCriteria: []string{"Focused proof passes"},
		Ambiguities:        []string{"Does the old caller require compatibility?"},
	}
	source := acceptedIntentProjection{
		ArtifactID: 41, JobID: 7, StepID: 11, JobGeneration: 1,
		LedgerID:      "ledger_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PayloadSHA256: strings.Repeat("b", 64), LedgerStart: 4,
	}
	projection, commands, err := buildAcceptedIntentProjection(source, intent)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ObjectiveNodeID == taskstate.NodeID(intent.Objectives[0].ID) ||
		!strings.HasPrefix(string(projection.ObjectiveNodeID), acceptedIntentObjectivePrefix) {
		t.Fatalf("objective ID %q reused or exposed the model ID", projection.ObjectiveNodeID)
	}
	if len(projection.Items) != 3 || len(commands) != 6 || projection.LedgerEnd != 10 {
		t.Fatalf("projection items/commands/end=%d/%d/%d", len(projection.Items), len(commands), projection.LedgerEnd)
	}
	for _, command := range commands {
		if _, forbidden := command.(taskstate.AssignNodeStepCommand); forbidden {
			t.Fatal("accepted intent projection assigned a task node to a generation step")
		}
	}
	constraint := commands[2].(taskstate.AddEntryCommand)
	if constraint.Actor != taskstate.AuthorityCode || constraint.Kind != taskstate.EntryConstraint {
		t.Fatalf("constraint authority=%q kind=%q", constraint.Actor, constraint.Kind)
	}
	question := commands[3].(taskstate.AddEntryCommand)
	if question.Actor != taskstate.AuthorityModelProposal || question.Kind != taskstate.EntryQuestion {
		t.Fatalf("ambiguity authority=%q kind=%q", question.Actor, question.Kind)
	}
	wantURI := "artifact://job/7/artifact/41/intent/ambiguity/0"
	if len(question.Refs) != 1 || question.Refs[0].URI != wantURI ||
		question.Refs[0].Hash != source.PayloadSHA256 || question.Refs[0].Relation != taskstate.RefSource {
		t.Fatalf("ambiguity source ref=%+v want URI %q", question.Refs, wantURI)
	}
}
