package codex

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestResumeIgnoresOnlyTheExactCompletedPriorTurnUsageSnapshot(t *testing.T) {
	call := codingagent.Assignment{Role: codingagent.Execute, Session: &codingagent.ExecutionSession{ThreadID: "thread", TurnID: "old-turn", State: "idle"}}
	for _, test := range []struct {
		name, method, thread, turn string
		skip                       bool
	}{
		{"captured resume snapshot", "thread/tokenUsage/updated", "thread", "old-turn", true},
		{"current usage", "thread/tokenUsage/updated", "thread", "new-turn", false},
		{"foreign thread", "thread/tokenUsage/updated", "foreign", "old-turn", false},
		{"foreign turn", "thread/tokenUsage/updated", "thread", "foreign", false},
		{"old command event", "item/completed", "thread", "old-turn", false},
		{"old completion", "turn/completed", "thread", "old-turn", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"method": test.method, "params": map[string]any{"threadId": test.thread, "turnId": test.turn}})
			frame := decodeAgentTestFrame(t, string(raw))
			if resumedUsageSnapshot(call, frame) != test.skip {
				t.Fatal("history exception escaped its exact usage-snapshot boundary")
			}
			if !test.skip && test.turn != "new-turn" {
				if err := validateAgentFrameBinding(frame, &codingagent.Result{ThreadID: "thread", TurnID: "new-turn"}); err == nil {
					t.Fatal("foreign activity bypassed assignment binding")
				}
			}
		})
	}
	frame := decodeAgentTestFrame(t, `{"method":"thread/tokenUsage/updated","params":{"threadId":"thread","turnId":"old-turn"}}`)
	for _, state := range []string{"running", "dispatching"} {
		call.Session.State = state
		if resumedUsageSnapshot(call, frame) {
			t.Fatal("active/uncertain turn usage was silently discarded")
		}
	}
	call.Session = nil
	if resumedUsageSnapshot(call, frame) {
		t.Fatal("fresh thread acquired a history exception")
	}
}
