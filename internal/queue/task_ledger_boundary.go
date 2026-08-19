package queue

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
)

const replanFeedbackEntryPrefix = "entry:user-feedback:replan:generation:"

// taskCommandBoundary is sealed inside queue so callers of Repository.ApplyTaskCommand
// cannot manufacture queue-owned lifecycle authority.
type taskCommandBoundary interface {
	validate(taskstate.Command) error
	queueTaskCommandBoundary()
}

type generalTaskCommandBoundary struct{}

func (generalTaskCommandBoundary) queueTaskCommandBoundary() {}

func (generalTaskCommandBoundary) validate(command taskstate.Command) error {
	if reservedTaskCommandMutation(command) {
		return fmt.Errorf(
			"%w: canonical task authority is owned by the queue lifecycle",
			taskstate.ErrAuthorityDenied,
		)
	}
	return nil
}

type queueOwnedTaskCommandBoundary struct{}

func (queueOwnedTaskCommandBoundary) queueTaskCommandBoundary()        {}
func (queueOwnedTaskCommandBoundary) validate(taskstate.Command) error { return nil }

func reservedTaskCommandMutation(command taskstate.Command) bool {
	switch typed := command.(type) {
	case taskstate.TerminalFailNodeCommand:
		return true
	case *taskstate.TerminalFailNodeCommand:
		return typed != nil
	case taskstate.SupersedeNodeGenerationCommand:
		return true
	case *taskstate.SupersedeNodeGenerationCommand:
		return typed != nil
	case taskstate.AddNodeCommand:
		// A code-owned objective must be able to attach beneath the canonical
		// queue-owned root. Referencing that root as a parent does not mutate
		// it; only manufacturing a reserved node identity or claiming one as an
		// objective remains queue-lifecycle authority.
		return reservedTaskNodeID(typed.ID) || reservedTaskNodeID(typed.ObjectiveID)
	case *taskstate.AddNodeCommand:
		return typed != nil && (reservedTaskNodeID(typed.ID) || reservedTaskNodeID(typed.ObjectiveID))
	case taskstate.AddEdgeCommand:
		return reservedTaskEdgeID(typed.ID) || reservedTaskNodeID(typed.From) || reservedTaskNodeID(typed.To)
	case *taskstate.AddEdgeCommand:
		return typed != nil && (reservedTaskEdgeID(typed.ID) ||
			reservedTaskNodeID(typed.From) || reservedTaskNodeID(typed.To))
	case taskstate.AssignNodeStepCommand:
		return reservedTaskNodeID(typed.NodeID)
	case *taskstate.AssignNodeStepCommand:
		return typed != nil && reservedTaskNodeID(typed.NodeID)
	case taskstate.TransitionNodeCommand:
		return reservedTaskNodeID(typed.NodeID)
	case *taskstate.TransitionNodeCommand:
		return typed != nil && reservedTaskNodeID(typed.NodeID)
	case taskstate.AddEntryCommand:
		return reservedTaskEntryID(typed.ID) || reservedTaskNodeID(typed.ScopeNodeID)
	case *taskstate.AddEntryCommand:
		return typed != nil && (reservedTaskEntryID(typed.ID) || reservedTaskNodeID(typed.ScopeNodeID))
	case taskstate.RejectEntryCommand:
		return reservedTaskEntryID(typed.EntryID)
	case *taskstate.RejectEntryCommand:
		return typed != nil && reservedTaskEntryID(typed.EntryID)
	case taskstate.ResolveEntryCommand:
		return reservedTaskEntryID(typed.EntryID)
	case *taskstate.ResolveEntryCommand:
		return typed != nil && reservedTaskEntryID(typed.EntryID)
	case taskstate.SupersedeEntryCommand:
		return reservedTaskEntryID(typed.EntryID) || reservedTaskEntryID(typed.ReplacementID)
	case *taskstate.SupersedeEntryCommand:
		return typed != nil && (reservedTaskEntryID(typed.EntryID) || reservedTaskEntryID(typed.ReplacementID))
	case taskstate.AcceptDecisionCommand:
		return reservedTaskEntryID(typed.CandidateID) || reservedTaskEntryID(typed.AcceptedEntryID)
	case *taskstate.AcceptDecisionCommand:
		return typed != nil && (reservedTaskEntryID(typed.CandidateID) || reservedTaskEntryID(typed.AcceptedEntryID))
	default:
		return false
	}
}

func reservedTaskEntryID(id taskstate.EntryID) bool {
	return id == initialUserInstructionEntryID ||
		strings.HasPrefix(string(id), replanFeedbackEntryPrefix) ||
		strings.HasPrefix(string(id), retiredCognitionEntryPrefix)
}

func reservedTaskNodeID(id taskstate.NodeID) bool {
	return id == initialTaskRootNodeID ||
		strings.HasPrefix(string(id), retiredCognitionObligationNodePrefix)
}

func reservedTaskEdgeID(id taskstate.EdgeID) bool {
	return strings.HasPrefix(string(id), retiredCognitionObligationEdgePrefix)
}
