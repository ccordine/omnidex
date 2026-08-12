package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestGeneralTaskBoundaryReservesQueueOwnedAuthority(t *testing.T) {
	commands := []taskstate.Command{
		taskstate.TerminalFailNodeCommand{
			NodeID: "task:ordinary", Reason: "terminal proof",
			Proof: taskstate.Ref{
				URI: "cognition:episode/1/terminal", Version: "1",
				Hash: strings.Repeat("b", 64), Relation: taskstate.RefVerifies,
			},
		},
		taskstate.SupersedeNodeGenerationCommand{NodeIDs: []taskstate.NodeID{"task:ordinary"}},
		taskstate.TransitionNodeCommand{NodeID: retiredCognitionObligationNodePrefix + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		taskstate.AddEdgeCommand{ID: retiredCognitionObligationEdgePrefix + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		taskstate.AddEntryCommand{ID: retiredCognitionEntryPrefix + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		taskstate.AddNodeCommand{ID: initialTaskRootNodeID},
		taskstate.TransitionNodeCommand{NodeID: initialTaskRootNodeID},
		taskstate.AssignNodeStepCommand{NodeID: initialTaskRootNodeID},
		taskstate.AddEntryCommand{ID: initialUserInstructionEntryID},
		taskstate.ResolveEntryCommand{EntryID: initialUserInstructionEntryID},
		taskstate.RejectEntryCommand{EntryID: initialUserInstructionEntryID},
		taskstate.SupersedeEntryCommand{ReplacementID: initialUserInstructionEntryID},
		taskstate.AcceptDecisionCommand{AcceptedEntryID: initialUserInstructionEntryID},
		taskstate.AddEntryCommand{ID: replanFeedbackEntryID(2)},
		taskstate.AddNodeCommand{ID: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.AddNodeCommand{ID: "task:new", ParentID: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.AddEdgeCommand{ID: taskstate.EdgeID(acceptedIntentEdgePrefix + "abc")},
		taskstate.AddEdgeCommand{ID: "edge:new", To: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.TransitionNodeCommand{NodeID: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.AssignNodeStepCommand{NodeID: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.AddEntryCommand{ID: taskstate.EntryID(acceptedIntentEntryPrefix + "constraint:abc")},
		taskstate.AddEntryCommand{ID: "entry:new", ScopeNodeID: taskstate.NodeID(acceptedIntentObjectivePrefix + "abc")},
		taskstate.ResolveEntryCommand{EntryID: taskstate.EntryID(acceptedIntentEntryPrefix + "ambiguity:abc")},
	}
	for _, command := range commands {
		if err := (generalTaskCommandBoundary{}).validate(command); !errors.Is(err, taskstate.ErrAuthorityDenied) {
			t.Fatalf("command %T reservation error=%v", command, err)
		}
		if err := (queueOwnedTaskCommandBoundary{}).validate(command); err != nil {
			t.Fatalf("queue-owned command %T rejected: %v", command, err)
		}
	}
	ordinary := taskstate.AddNodeCommand{ID: "task:ordinary"}
	if err := (generalTaskCommandBoundary{}).validate(ordinary); err != nil {
		t.Fatalf("ordinary command rejected: %v", err)
	}
}
