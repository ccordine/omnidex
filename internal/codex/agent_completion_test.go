package codex

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Commands and prose are intermediate output. The native terminal turn event
// completes the entire call, after which its final message can be parsed.
func TestNativeRunWaitsForWholeTurnCompletion(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
			frames := []string{
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"command","type":"commandExecution","status":"completed","exitCode":0}}}`,
				`{"method":"item/commandExecution/outputDelta","params":{"threadId":"thread","turnId":"turn","itemId":"command","delta":"last command output"}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"progress","type":"agentMessage","phase":"commentary","text":"Checking the result."}}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage","phase":"final_answer","text":"Work finished."}}}`,
				`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`,
			}
			for index, raw := range frames {
				var frame rpcFrame
				if err := json.Unmarshal([]byte(raw), &frame); err != nil {
					t.Fatal(err)
				}
				done, err := consumeAgentFrame(frame, json.RawMessage(raw), &result, role, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantDone := index == len(frames)-1
				if done != wantDone || result.Completed != wantDone {
					t.Fatalf("frame %d: completed=%v, want %v", index, done, wantDone)
				}
			}
			if string(result.Content) != `"Work finished."` {
				t.Fatalf("parser input is not the final message: %s", result.Content)
			}
		})
	}
}
