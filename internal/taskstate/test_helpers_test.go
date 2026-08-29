package taskstate

import (
	"encoding/json"
	"testing"
)

func withTestCommandID(t *testing.T, command Command) Command {
	t.Helper()
	raw, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewCommandID(t.Name(), string(raw))
	if err != nil {
		t.Fatal(err)
	}
	switch value := command.(type) {
	case AddNodeCommand:
		value.CommandID = id
		return value
	case AddEdgeCommand:
		value.CommandID = id
		return value
	case AddEntryCommand:
		value.CommandID = id
		return value
	case RejectEntryCommand:
		value.CommandID = id
		return value
	case ResolveEntryCommand:
		value.CommandID = id
		return value
	case SupersedeEntryCommand:
		value.CommandID = id
		return value
	case PromoteReadyNodesCommand:
		value.CommandID = id
		return value
	case AssignNodeStepCommand:
		value.CommandID = id
		return value
	case TransitionNodeCommand:
		value.CommandID = id
		return value
	case TerminalFailNodeCommand:
		value.CommandID = id
		return value
	case SupersedeNodeGenerationCommand:
		value.CommandID = id
		return value
	case CloseLedgerCommand:
		value.CommandID = id
		return value
	default:
		t.Fatalf("unsupported test command %T", command)
		return nil
	}
}
