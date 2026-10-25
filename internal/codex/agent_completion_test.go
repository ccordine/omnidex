package codex

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestTerminalTurnClosesUnmatchedReasoningOnly(t *testing.T) {
	for _, fixture := range []struct {
		name, status, turnID string
		unfinishedCommand    bool
		wantDone             bool
		wantOpen             int
	}{
		{"completed reasoning", "completed", "turn", false, true, 0},
		{"unfinished command", "completed", "turn", true, true, 1},
		{"failed turn", "failed", "turn", false, false, 1},
		{"interrupted turn", "interrupted", "turn", false, false, 1},
		{"different turn", "completed", "other", false, false, 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
			open, closed := map[string]string{}, map[string]string{}
			inputs := []json.RawMessage{
				json.RawMessage(`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"reason","type":"reasoning"}}}`),
				json.RawMessage(`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution"}}}`),
			}
			if !fixture.unfinishedCommand {
				inputs = append(inputs, json.RawMessage(`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","status":"completed","exitCode":0}}}`))
			}
			terminal, err := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{
				"threadId": "thread", "turn": map[string]any{"id": fixture.turnID, "status": fixture.status,
					"items": []map[string]string{{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": "Verified retained work."}}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			inputs = append(inputs, terminal)
			for i, raw := range inputs {
				var frame rpcFrame
				if err := json.Unmarshal(raw, &frame); err != nil {
					t.Fatal(err)
				}
				done, err := consumeAgentFrameTracked(frame, raw, &result, codingagent.Execute, open, closed, nil)
				if i != len(inputs)-1 {
					if done || err != nil {
						t.Fatalf("nonterminal frame ended assignment: done=%v err=%v", done, err)
					}
					if _, present := open["reason"]; !present {
						t.Fatal("reasoning closed before terminal confirmation")
					}
					continue
				}
				if done != fixture.wantDone || (err == nil) != fixture.wantDone || len(open) != fixture.wantOpen {
					t.Fatalf("terminal handling: done=%v err=%v open=%v", done, err, open)
				}
			}
			if len(result.Evidence) != len(inputs) {
				t.Fatal("lost original receipts")
			}
			for i, raw := range inputs {
				if !bytes.Equal(raw, result.Evidence[i]) {
					t.Fatalf("changed original receipt %d", i)
				}
			}
		})
	}
}
