package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestTaskCommandStepTargetClassifiesEveryRegisteredCommand(t *testing.T) {
	stepID := int64(41)
	cases := []struct {
		name    string
		command taskstate.Command
		want    *int64
	}{
		{name: "add node", command: taskstate.AddNodeCommand{CreatedStepID: &stepID}, want: &stepID},
		{name: "add edge", command: taskstate.AddEdgeCommand{}},
		{name: "add entry", command: taskstate.AddEntryCommand{CreatedStepID: &stepID}, want: &stepID},
		{name: "reject entry", command: taskstate.RejectEntryCommand{}},
		{name: "resolve entry", command: taskstate.ResolveEntryCommand{}},
		{name: "supersede entry", command: taskstate.SupersedeEntryCommand{}},
		{name: "promote ready", command: taskstate.PromoteReadyNodesCommand{}},
		{name: "assign step", command: taskstate.AssignNodeStepCommand{StepID: stepID}, want: &stepID},
		{name: "transition node", command: taskstate.TransitionNodeCommand{CompletedStepID: &stepID}, want: &stepID},
		{name: "close ledger", command: taskstate.CloseLedgerCommand{StepID: &stepID}, want: &stepID},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := taskCommandStepTarget(testCase.command)
			if err != nil {
				t.Fatal(err)
			}
			if testCase.want == nil {
				if got != nil {
					t.Fatalf("step target=%d, want none", *got)
				}
				return
			}
			if got == nil || *got != *testCase.want {
				t.Fatalf("step target=%v, want %d", got, *testCase.want)
			}
		})
	}
}
